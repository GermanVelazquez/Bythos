package com.bythos.movil

import android.content.Intent
import android.os.Build
import android.os.Bundle
import android.view.View
import android.widget.Button
import android.widget.LinearLayout
import android.widget.TextView

/**
 * Entry screen: scan the PC's QR, or paste the link when scanning is unavailable.
 * After a valid QR it asks the PC to pair (code -> request), waits for the approval
 * on the PC and stores the token in the Keystore-backed store.
 */
class PairingActivity : ActividadRedLocal() {

    private lateinit var estado: TextView
    private lateinit var pegado: View

    /** Last valid QR, the target of the pairing handshake. */
    var qr: QrEmparejamiento? = null
        private set

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        estado = TextView(this).apply { text = getString(R.string.emparejar_instruccion) }
        pegado = PegadoManual.vista(this, ::alTexto).apply { visibility = View.GONE }
        val escanear = Button(this).apply {
            text = getString(R.string.escanear_qr)
            setOnClickListener { Escaner.escanear(this@PairingActivity, ::alTexto, ::mostrarPegado) }
        }
        val pegar = Button(this).apply {
            text = getString(R.string.pegar_enlace)
            setOnClickListener { mostrarPegado() }
        }
        val enviar = Button(this).apply {
            text = getString(R.string.enviar_archivo)
            setOnClickListener { startActivity(Intent(this@PairingActivity, ShareReceiverActivity::class.java)) }
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
                addView(escanear)
                addView(pegar)
                addView(enviar)
                addView(pegado)
            },
        )
    }

    private fun mostrarPegado() {
        pegado.visibility = View.VISIBLE
    }

    private fun alTexto(texto: String) {
        try {
            val leido = parsearQr(texto)
            qr = leido
            estado.text = getString(R.string.qr_leido, leido.nombrePC.ifEmpty { leido.host })
            conPermisoRedLocal { emparejar(leido) }
        } catch (e: QrInvalidoException) {
            estado.text = getString(R.string.qr_invalido, e.message)
        }
    }

    /** Trades the QR code for a token: request, wait for the PC approval, store the token encrypted. */
    private fun emparejar(leido: QrEmparejamiento) {
        Thread {
            try {
                val api = EmparejarApi(ApiClient(leido.host, leido.puerto, leido.huella))
                val solicitud = api.solicitar(leido.codigo, Build.MODEL)
                mostrar(getString(R.string.esperando_aprobacion, solicitud.sas))
                val limite = System.currentTimeMillis() + ESPERA_MAX_MS
                while (System.currentTimeMillis() < limite) {
                    val e = api.estado(solicitud.id)
                    if (e.estado == "rechazado") return@Thread mostrar(getString(R.string.rechazado))
                    if (e.estado == "aprobado" && e.token != null) {
                        val prefs = getSharedPreferences("bythos", MODE_PRIVATE)
                        KeystoreTokenStore.deContexto(prefs).guardar(e.token)
                        // Non-secret pairing target, needed to reconnect with the same pin.
                        prefs.edit().putString("host", leido.host).putInt("puerto", leido.puerto)
                            .putString("huella", leido.huella).apply()
                        return@Thread mostrar(getString(R.string.emparejado, leido.nombrePC.ifEmpty { leido.host }))
                    }
                    Thread.sleep(SONDEO_MS)
                }
            } catch (e: Exception) {
                mostrar(textoFallo(e))
            }
        }.start()
    }

    private fun mostrar(texto: String) = runOnUiThread { estado.text = texto }

    private companion object {
        const val SONDEO_MS = 2_000L
        const val ESPERA_MAX_MS = 10 * 60_000L
    }
}
