package com.bythos.movil

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.content.Context
import android.content.pm.ServiceInfo
import android.net.Uri
import androidx.work.BackoffPolicy
import androidx.work.Constraints
import androidx.work.ExistingWorkPolicy
import androidx.work.ForegroundInfo
import androidx.work.NetworkType
import androidx.work.OneTimeWorkRequestBuilder
import androidx.work.WorkManager
import androidx.work.Worker
import androidx.work.WorkerParameters
import androidx.work.workDataOf
import org.json.JSONObject
import java.io.FileNotFoundException
import java.io.IOException
import java.io.InputStream
import java.security.MessageDigest
import java.util.concurrent.TimeUnit

class SubidaCreada(val id: String, val offset: Long)
class EstadoRemoto(val estado: String, val offset: Long, val error: String)

/** What the upload loop needs from the PC; the real one is [ServidorHttp], tests use a fake. */
interface Servidor {
    fun crear(e: Envio): SubidaCreada
    fun estado(id: String): EstadoRemoto
    fun parte(id: String, offset: Long, sha256: String, datos: ByteArray): Long
    fun enlace(carpetaId: Long, url: String)
}

/** Opens the bytes of an outbox item (content or file URI). */
fun interface Fuente {
    fun abrir(uri: String): InputStream
}

class ServidorHttp(private val api: ApiClient) : Servidor {
    override fun crear(e: Envio): SubidaCreada {
        val cuerpo = JSONObject().put("cliente_id", e.clienteId).put("carpeta_id", e.carpetaId)
            .put("nombre", e.nombre).put("tamano", e.tamano).put("sha256", e.sha256)
        val j = JSONObject(api.postJson("/v1/subidas", cuerpo.toString()).cuerpo)
        return SubidaCreada(j.getString("id"), j.getLong("offset"))
    }

    override fun estado(id: String): EstadoRemoto {
        val j = JSONObject(api.postJson("/v1/subidas/$id/estado", "{}").cuerpo)
        return EstadoRemoto(j.getString("estado"), j.getLong("offset"), j.optString("error"))
    }

    override fun parte(id: String, offset: Long, sha256: String, datos: ByteArray): Long {
        val cab = mapOf("X-Offset" to offset.toString(), "X-Chunk-SHA256" to sha256, "Content-Type" to "application/octet-stream")
        return JSONObject(api.postBytes("/v1/subidas/$id/partes", datos, cab).cuerpo).getLong("offset")
    }

    override fun enlace(carpetaId: Long, url: String) {
        LinksApi(api).enviar(carpetaId, url)
    }
}

enum class Resultado { LISTO, REINTENTAR, FALLO }

private class Detenido : Exception()
private class FuenteInaccesible(mensaje: String) : Exception(mensaje)
private class Rechazo(val codigo: String) : Exception(codigo)

fun sha256Hex(b: ByteArray, largo: Int = b.size): String =
    MessageDigest.getInstance("SHA-256").apply { update(b, 0, largo) }.digest().joinToString("") { "%02x".format(it) }

/**
 * The upload loop for one outbox row. The PC's `/estado` is the source of truth on every
 * (re)start, so a kill or network loss resumes from the last acknowledged offset.
 */
class Subidor(
    private val servidor: Servidor,
    private val repo: EnviosRepo,
    private val fuente: Fuente,
    private val cancelado: () -> Boolean = { false },
    private val pausa: (Long) -> Unit = { Thread.sleep(it) },
    private val tamanoParte: Int = PARTE,
) {
    private lateinit var e: Envio

    fun ejecutar(id: Long): Resultado {
        e = repo.obtener(id) ?: return Resultado.FALLO
        when (e.estado) {
            EstadoEnvio.LISTO -> return Resultado.LISTO
            EstadoEnvio.ERROR, EstadoEnvio.SIN_EMPAREJAR -> return Resultado.FALLO
            EstadoEnvio.PENDIENTE -> e = repo.guardar(e.con(EventoEnvio.INICIAR))
            EstadoEnvio.SUBIENDO -> Unit // the process died mid-upload
        }
        return try {
            if (e.tipo == Envio.TIPO_LINK) servidor.enlace(e.carpetaId, e.uri) else subirArchivo()
            repo.guardar(e.con(EventoEnvio.COMPLETAR))
            Resultado.LISTO
        } catch (x: Detenido) {
            parar()
        } catch (x: ErrorApi) {
            when {
                x.status == 401 -> terminar(EventoEnvio.DESEMPAREJAR, x.codigo)
                x.status >= 500 || x.codigo in REINTENTABLES -> parar()
                else -> terminar(EventoEnvio.FALLAR, x.message ?: x.codigo)
            }
        } catch (x: Rechazo) {
            terminar(EventoEnvio.FALLAR, x.codigo)
        } catch (x: FuenteInaccesible) {
            terminar(EventoEnvio.FALLAR, x.message ?: "fuente_inaccesible")
        } catch (x: IOException) {
            parar()
        }
    }

    private fun parar(): Resultado {
        repo.guardar(e.con(EventoEnvio.DETENER))
        return Resultado.REINTENTAR
    }

    private fun terminar(evento: EventoEnvio, error: String): Resultado {
        repo.guardar(e.con(evento, error))
        return Resultado.FALLO
    }

    private fun subirArchivo() {
        if (e.sha256 == null) e = repo.guardar(e.copy(sha256 = hashArchivo()))
        sincronizar()
        enviarPartes()
        esperarVerificacion()
    }

    private fun sincronizar() {
        val id = e.subidaId ?: return crear()
        val r = try {
            servidor.estado(id)
        } catch (x: ErrorApi) {
            if (x.codigo != "subida_no_encontrada") throw x
            e = repo.guardar(e.copy(subidaId = null, offset = 0))
            return crear()
        }
        when (r.estado) {
            "error" -> throw Rechazo(r.error)
            "recibiendo" -> e = repo.guardar(e.copy(offset = r.offset))
            else -> e = repo.guardar(e.copy(offset = e.tamano)) // verificando / listo: nothing left to send
        }
    }

    private fun crear() {
        val c = servidor.crear(e)
        e = repo.guardar(e.copy(subidaId = c.id, offset = c.offset))
    }

    private fun enviarPartes() {
        var fallosHash = 0
        var desajustes = 0
        while (e.offset < e.tamano) {
            if (cancelado()) throw Detenido()
            val datos = leer(e.offset, minOf(tamanoParte.toLong(), e.tamano - e.offset).toInt())
            try {
                val nuevo = servidor.parte(e.subidaId!!, e.offset, sha256Hex(datos), datos)
                e = repo.guardar(e.copy(offset = nuevo))
                fallosHash = 0
                desajustes = 0
            } catch (x: ErrorApi) {
                when {
                    x.codigo == "offset_invalido" && x.offset != null && ++desajustes <= MAX_REINTENTOS ->
                        e = repo.guardar(e.copy(offset = x.offset)) // resume from the PC's offset
                    x.codigo == "hash_parte_invalido" && ++fallosHash <= MAX_REINTENTOS -> Unit // same chunk again
                    else -> throw x
                }
            }
        }
    }

    private fun esperarVerificacion() {
        repeat(SONDEOS) {
            val r = servidor.estado(e.subidaId!!)
            if (r.estado == "listo") return
            if (r.estado == "error") throw Rechazo(r.error)
            if (cancelado()) throw Detenido()
            pausa(SONDEO_MS)
        }
        throw Detenido() // still verifying: WorkManager comes back and asks again
    }

    private fun leer(offset: Long, n: Int): ByteArray = abrir { s ->
        saltar(s, offset)
        val b = ByteArray(n)
        var l = 0
        while (l < n) {
            val r = s.read(b, l, n - l)
            if (r < 0) throw FuenteInaccesible("fuente_mas_corta")
            l += r
        }
        b
    }

    private fun hashArchivo(): String = abrir { s ->
        val md = MessageDigest.getInstance("SHA-256")
        val b = ByteArray(64 * 1024)
        var total = 0L
        while (true) {
            val r = s.read(b)
            if (r < 0) break
            md.update(b, 0, r)
            total += r
        }
        if (total != e.tamano) throw FuenteInaccesible("el archivo cambió")
        md.digest().joinToString("") { "%02x".format(it) }
    }

    /** Source failures (deleted, revoked grant) are permanent for this item, unlike network ones. */
    private fun <T> abrir(bloque: (InputStream) -> T): T = try {
        fuente.abrir(e.uri).use(bloque)
    } catch (x: IOException) {
        throw FuenteInaccesible("fuente_inaccesible")
    } catch (x: SecurityException) {
        throw FuenteInaccesible("fuente_inaccesible")
    }

    private fun saltar(s: InputStream, n: Long) {
        var resto = n
        while (resto > 0) {
            val k = s.skip(resto)
            if (k > 0) resto -= k else if (s.read() < 0) throw FuenteInaccesible("fuente_mas_corta") else resto--
        }
    }

    companion object {
        const val PARTE = 4 shl 20 // well under the PC's 8 MiB chunk_max
        const val MAX_REINTENTOS = 3
        const val SONDEOS = 60
        const val SONDEO_MS = 1_000L
        private val REINTENTABLES = setOf("subida_ocupada", "demasiadas_subidas")
    }
}

/** WorkManager entry: one unique work per outbox row, Wi-Fi/unmetered only, run as a dataSync foreground service. */
class UploadWorker(contexto: Context, params: WorkerParameters) : Worker(contexto, params) {

    @Volatile private var detenido = false

    override fun doWork(): Result {
        val id = inputData.getLong(ID, 0)
        val repo = EnviosDbHelper(applicationContext)
        anunciar(id)
        val api = clienteEmparejado(applicationContext)
        if (api == null) {
            repo.obtener(id)?.takeIf { transicion(it.estado, EventoEnvio.DESEMPAREJAR) != null }
                ?.let { repo.guardar(it.con(EventoEnvio.DESEMPAREJAR)) }
            return Result.failure()
        }
        val fuente = Fuente { uri ->
            applicationContext.contentResolver.openInputStream(Uri.parse(uri)) ?: throw FileNotFoundException(uri)
        }
        // On stop the loop persists the acknowledged offset and goes back to PENDIENTE.
        return when (Subidor(ServidorHttp(api), repo, fuente, cancelado = { detenido }).ejecutar(id)) {
            Resultado.LISTO -> Result.success()
            Resultado.REINTENTAR -> Result.retry()
            Resultado.FALLO -> Result.failure()
        }
    }

    override fun onStopped() {
        detenido = true
    }

    private fun anunciar(id: Long) {
        val nm = applicationContext.getSystemService(NotificationManager::class.java)
        nm.createNotificationChannel(NotificationChannel(CANAL, "Envíos a la PC", NotificationManager.IMPORTANCE_LOW))
        val n = Notification.Builder(applicationContext, CANAL)
            .setSmallIcon(android.R.drawable.stat_sys_upload).setContentTitle("Enviando a Bythos").setOngoing(true).build()
        try {
            setForegroundAsync(ForegroundInfo(NOTIF_BASE + (id % 1000).toInt(), n, ServiceInfo.FOREGROUND_SERVICE_TYPE_DATA_SYNC))
        } catch (e: Exception) {
            // The OS may refuse a foreground start from the background; the upload still runs best-effort.
        }
    }

    companion object {
        private const val ID = "envio_id"
        private const val CANAL = "envios"
        private const val NOTIF_BASE = 1000

        fun encolar(contexto: Context, id: Long) {
            val pedido = OneTimeWorkRequestBuilder<UploadWorker>()
                .setInputData(workDataOf(ID to id))
                .setConstraints(Constraints.Builder().setRequiredNetworkType(NetworkType.UNMETERED).build())
                .setBackoffCriteria(BackoffPolicy.EXPONENTIAL, 30, TimeUnit.SECONDS)
                .build()
            WorkManager.getInstance(contexto).enqueueUniqueWork("envio-$id", ExistingWorkPolicy.KEEP, pedido)
        }
    }
}
