package com.bythos.movil

import android.app.AlertDialog
import android.content.Intent
import android.net.Uri
import android.os.Bundle
import android.provider.OpenableColumns
import android.widget.TextView
import androidx.lifecycle.LiveData
import androidx.lifecycle.Observer
import androidx.work.WorkInfo
import androidx.work.WorkManager
import java.io.File
import java.util.UUID

/** An item to send: a content URI or a pasted link. */
private class Pendiente(val uri: Uri?, val url: String?)

private class SinEspacio : Exception()

/** Extracts the first http(s) URL from shared text. */
fun extraerUrl(texto: String?): String? = texto?.let { URL_RE.find(it)?.value }
private val URL_RE = Regex("https?://\\S+")

/**
 * Target of ACTION_SEND / ACTION_SEND_MULTIPLE, and of the in-app "send a file" button
 * (ACTION_OPEN_DOCUMENT). Asks for the PC folder, then queues one outbox row per item.
 */
class ShareReceiverActivity : ActividadRedLocal() {

    private lateinit var estado: TextView
    private var pendientes: List<Pendiente> = emptyList()
    private val lineas = linkedMapOf<Long, String>()
    private val observadores = mutableListOf<Pair<LiveData<List<WorkInfo>>, Observer<List<WorkInfo>>>>()

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        estado = TextView(this).apply { setPadding(48, 96, 48, 48) }
        setContentView(estado)
        when (intent.action) {
            Intent.ACTION_SEND -> {
                val uri = flujo(Intent.EXTRA_STREAM)
                pendientes = listOf(Pendiente(uri, if (uri == null) extraerUrl(intent.getStringExtra(Intent.EXTRA_TEXT)) else null))
                    .filter { it.uri != null || it.url != null }
            }
            Intent.ACTION_SEND_MULTIPLE ->
                pendientes = (intent.getParcelableArrayListExtra(Intent.EXTRA_STREAM, Uri::class.java) ?: arrayListOf())
                    .map { Pendiente(it, null) }
            else -> return startActivityForResult(
                Intent(Intent.ACTION_OPEN_DOCUMENT).addCategory(Intent.CATEGORY_OPENABLE).setType("*/*")
                    .putExtra(Intent.EXTRA_ALLOW_MULTIPLE, true)
                    .addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION or Intent.FLAG_GRANT_PERSISTABLE_URI_PERMISSION),
                PEDIDO_DOCUMENTO,
            )
        }
        elegirCarpeta()
    }

    @Deprecated("Plain Activity result API keeps this screen dependency-free")
    override fun onActivityResult(requestCode: Int, resultCode: Int, data: Intent?) {
        super.onActivityResult(requestCode, resultCode, data)
        val clip = data?.clipData
        val uris = if (clip != null) List(clip.itemCount) { clip.getItemAt(it).uri } else listOfNotNull(data?.data)
        pendientes = uris.map { Pendiente(it, null) }
        elegirCarpeta()
    }

    private fun flujo(clave: String): Uri? = intent.getParcelableExtra(clave, Uri::class.java)

    private fun elegirCarpeta() {
        if (pendientes.isEmpty()) return avisar(getString(R.string.nada_que_enviar))
        avisar(getString(R.string.cargando_carpetas))
        conPermisoRedLocal {
            Thread {
                try {
                    val api = clienteEmparejado(this) ?: throw ErrorApi(401, "token_invalido", "sin emparejar")
                    val carpetas = FoldersApi(api).listar()
                    runOnUiThread {
                        AlertDialog.Builder(this).setTitle(R.string.elegir_carpeta)
                            .setItems(carpetas.map { it.nombre }.toTypedArray()) { _, i -> encolar(carpetas[i].id) }
                            .setOnCancelListener { finish() }.show()
                    }
                } catch (e: Exception) {
                    avisar(textoFallo(e))
                }
            }.start()
        }
    }

    private fun encolar(carpetaId: Long) {
        Thread {
            val repo = EnviosDbHelper(this)
            try {
                val nuevos = mutableListOf<Pair<Long, String>>()
                for (p in pendientes) {
                    val envio = if (p.url != null) {
                        Envio(tipo = Envio.TIPO_LINK, uri = p.url, nombre = p.url, tamano = 0, carpetaId = carpetaId, clienteId = UUID.randomUUID().toString())
                    } else {
                        prepararArchivo(p.uri!!, carpetaId)
                    }
                    val guardado = repo.guardar(envio)
                    UploadWorker.encolar(this, guardado.id)
                    nuevos += guardado.id to guardado.nombre
                }
                avisar(getString(R.string.encolados, pendientes.size))
                runOnUiThread { nuevos.forEach { (id, nombre) -> observarEnvio(id, nombre) } }
            } catch (e: SinEspacio) {
                avisar(getString(R.string.sin_espacio))
            } catch (e: Exception) {
                avisar(getString(R.string.fallo_fuente))
            }
        }.start()
    }

    /** Shows each queued item's phase and percent from its unique work's progress; removed in onDestroy. */
    private fun observarEnvio(id: Long, nombre: String) {
        val datos = WorkManager.getInstance(this).getWorkInfosForUniqueWorkLiveData("envio-$id")
        val observador = Observer<List<WorkInfo>> { infos ->
            val w = infos.firstOrNull() ?: return@Observer
            val fase = w.progress.getString(UploadWorker.FASE)?.let { Fase.valueOf(it) }
            lineas[id] = when {
                w.state == WorkInfo.State.SUCCEEDED -> "$nombre: ${getString(R.string.fase_listo)}"
                fase == null -> return@Observer
                fase == Fase.VERIFICANDO -> "$nombre: ${getString(UploadWorker.textoFase(fase))}"
                else -> "$nombre: ${getString(UploadWorker.textoFase(fase))} " +
                    "${UploadWorker.porcentaje(w.progress.getLong(UploadWorker.HECHOS, 0), w.progress.getLong(UploadWorker.TOTAL, 0))} %"
            }
            estado.text = lineas.values.joinToString("\n")
        }
        datos.observeForever(observador)
        observadores += datos to observador
    }

    override fun onDestroy() {
        observadores.forEach { (d, o) -> d.removeObserver(o) }
        super.onDestroy()
    }

    /** Keeps the content URI when the grant can be made persistent; otherwise copies it to cache. */
    private fun prepararArchivo(uri: Uri, carpetaId: Long): Envio {
        var nombre = uri.lastPathSegment ?: "archivo"
        var tamano = -1L
        contentResolver.query(uri, arrayOf(OpenableColumns.DISPLAY_NAME, OpenableColumns.SIZE), null, null, null)?.use {
            if (it.moveToFirst()) {
                it.getString(0)?.let { n -> nombre = n }
                if (!it.isNull(1)) tamano = it.getLong(1)
            }
        }
        var destino = uri
        val persistente = tamano >= 0 && try {
            contentResolver.takePersistableUriPermission(uri, Intent.FLAG_GRANT_READ_URI_PERMISSION)
            true
        } catch (e: SecurityException) {
            false // a one-shot share grant dies with this task: copy instead
        }
        if (!persistente) {
            if (tamano >= 0 && cacheDir.usableSpace < tamano + MARGEN_LIBRE) throw SinEspacio()
            val copia = File(cacheDir, "envio-${UUID.randomUUID()}")
            contentResolver.openInputStream(uri)!!.use { i -> copia.outputStream().use { o -> i.copyTo(o) } }
            tamano = copia.length()
            destino = Uri.fromFile(copia)
        }
        return Envio(tipo = Envio.TIPO_ARCHIVO, uri = destino.toString(), nombre = nombre, tamano = tamano, carpetaId = carpetaId, clienteId = UUID.randomUUID().toString())
    }

    private fun avisar(texto: String) = runOnUiThread { estado.text = texto }

    private companion object {
        const val PEDIDO_DOCUMENTO = 2
        const val MARGEN_LIBRE = 16L shl 20
    }
}
