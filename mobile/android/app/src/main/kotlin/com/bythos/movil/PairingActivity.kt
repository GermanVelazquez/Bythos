package com.bythos.movil

import android.content.Intent
import android.os.Build
import android.os.Bundle
import android.util.Log
import android.view.View
import android.widget.Button
import android.widget.LinearLayout
import android.widget.ProgressBar
import android.widget.TextView
import java.net.ConnectException
import java.net.SocketTimeoutException

/**
 * Entry screen: paired home when a pairing exists, otherwise scan the PC's QR (or paste the link
 * when scanning is unavailable). After a valid QR it asks the PC to pair (code -> request),
 * waits for the approval on the PC and stores the token in the Keystore-backed store.
 */
class PairingActivity : ActividadRedLocal() {

    private lateinit var estado: TextView
    private lateinit var pegado: View
    private lateinit var progreso: ProgressBar
    private lateinit var escanear: Button
    private lateinit var pegar: Button
    private lateinit var enviar: Button
    private lateinit var reemparejar: Button

    /** Last valid QR, the target of the pairing handshake. */
    var qr: QrEmparejamiento? = null
        private set

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        estado = TextView(this)
        progreso = ProgressBar(this).apply { isIndeterminate = true; visibility = View.GONE }
        pegado = PegadoManual.vista(this, ::alTexto).apply { visibility = View.GONE }
        escanear = Button(this).apply {
            text = getString(R.string.escanear_qr)
            setOnClickListener { Escaner.escanear(this@PairingActivity, ::alTexto, ::mostrarPegado) }
        }
        pegar = Button(this).apply {
            text = getString(R.string.pegar_enlace)
            setOnClickListener { mostrarPegado() }
        }
        enviar = Button(this).apply {
            text = getString(R.string.enviar_archivo)
            setOnClickListener { startActivity(Intent(this@PairingActivity, ShareReceiverActivity::class.java)) }
        }
        reemparejar = Button(this).apply {
            text = getString(R.string.volver_a_emparejar)
            // The paired view stays until a valid QR arrives: cancelling the scan keeps it.
            setOnClickListener { Escaner.escanear(this@PairingActivity, ::alTexto, ::mostrarPegado) }
        }
        setContentView(
            LinearLayout(this).apply {
                orientation = LinearLayout.VERTICAL
                setPadding(48, 96, 48, 48)
                addView(TextView(context).apply {
                    text = getString(R.string.emparejar_titulo)
                    textSize = 22f
                })
                addView(estado)
                addView(progreso)
                addView(enviar)
                addView(reemparejar)
                addView(escanear)
                addView(pegar)
                addView(pegado)
            },
        )
        if (clienteEmparejado(this) != null) modoEmparejado() else modoSinEmparejar()
    }

    private fun modoEmparejado() {
        val guardado = getSharedPreferences("bythos", MODE_PRIVATE).getString("nombrePC", null)
        estado.text = getString(R.string.emparejado_con, nombreVisiblePC(guardado, getString(R.string.pc_sin_nombre)))
        progreso.visibility = View.GONE
        pegado.visibility = View.GONE
        enviar.visibility = View.VISIBLE
        reemparejar.visibility = View.VISIBLE
        escanear.visibility = View.GONE
        pegar.visibility = View.GONE
    }

    private fun modoSinEmparejar() {
        estado.text = getString(R.string.emparejar_instruccion)
        enviar.visibility = View.GONE
        reemparejar.visibility = View.GONE
        escanear.visibility = View.VISIBLE
        pegar.visibility = View.VISIBLE
    }

    private fun mostrarPegado() {
        pegado.visibility = View.VISIBLE
    }

    private fun alTexto(texto: String) {
        try {
            val leido = parsearQr(texto)
            qr = leido
            modoSinEmparejar()
            estado.text = getString(R.string.qr_leido, nombreDe(leido))
            conPermisoRedLocal { emparejar(leido) }
        } catch (e: QrInvalidoException) {
            estado.text = getString(R.string.qr_invalido, e.message)
        }
    }

    private fun nombreDe(leido: QrEmparejamiento) = nombreVisiblePC(leido.nombrePC, getString(R.string.pc_sin_nombre))

    /** Trades the QR code for a token: request, wait for the PC approval, store the token encrypted. */
    private fun emparejar(leido: QrEmparejamiento) {
        mostrar(getString(R.string.conectando), girando = true)
        Thread {
            val t0 = System.currentTimeMillis()
            fun traza(paso: String) = Log.i("Bythos", "emparejar $paso +${System.currentTimeMillis() - t0} ms")
            try {
                traza("conectando")
                // Pairing answers are tiny and immediate: a short timeout detects a stale QR fast.
                val api = EmparejarApi(ApiClient(leido.host, leido.puerto, leido.huella, conexionMs = PAREO_CONEXION_MS, lecturaMs = PAREO_LECTURA_MS))
                val solicitud = api.solicitar(leido.codigo, Build.MODEL)
                traza("solicitar ok")
                mostrar(getString(R.string.esperando_aprobacion, solicitud.sas), girando = true)
                val limite = System.currentTimeMillis() + ESPERA_MAX_MS
                while (System.currentTimeMillis() < limite) {
                    val e = api.estado(solicitud.id)
                    traza("sondeo ${e.estado}")
                    if (e.estado == "rechazado") return@Thread mostrar(getString(R.string.rechazado))
                    if (e.estado == "aprobado" && e.token != null) {
                        val prefs = getSharedPreferences("bythos", MODE_PRIVATE)
                        KeystoreTokenStore.deContexto(prefs).guardar(e.token)
                        // Non-secret pairing target, needed to reconnect with the same pin.
                        prefs.edit().putString("host", leido.host).putInt("puerto", leido.puerto)
                            .putString("huella", leido.huella).putString("nombrePC", leido.nombrePC).apply()
                        mostrar(getString(R.string.emparejado, nombreDe(leido)))
                        return@Thread runOnUiThread { modoEmparejado() }
                    }
                    Thread.sleep(SONDEO_MS)
                }
            } catch (e: Exception) {
                traza("fallo ${e.javaClass.simpleName}")
                mostrar(textoFalloPareo(e))
            }
        }.start()
    }

    /** A timeout or refusal while pairing means the PC is not listening (stale QR); a pin mismatch stays distinct. */
    private fun textoFalloPareo(e: Exception): String {
        val sinRespuesta = generateSequence<Throwable>(e) { it.cause }.take(10).any { it is SocketTimeoutException || it is ConnectException }
        val inalcanzable = clasificarFallo(e, Build.VERSION.SDK_INT, permisoRedLocal()) == Fallo.INALCANZABLE
        return if (sinRespuesta && inalcanzable) getString(R.string.pc_no_recibe) else textoFallo(e)
    }

    private fun mostrar(texto: String, girando: Boolean = false) = runOnUiThread {
        estado.text = texto
        progreso.visibility = if (girando) View.VISIBLE else View.GONE
    }

    private companion object {
        const val SONDEO_MS = 2_000L
        const val ESPERA_MAX_MS = 10 * 60_000L
        const val PAREO_CONEXION_MS = 3_000
        const val PAREO_LECTURA_MS = 10_000
    }
}
