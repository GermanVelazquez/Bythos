package com.bythos.movil

import android.app.Activity
import android.os.Bundle
import android.view.View
import android.widget.Button
import android.widget.LinearLayout
import android.widget.TextView

/**
 * Entry screen: scan the PC's QR, or paste the link when scanning is unavailable.
 * The parsed pairing data (host, port, pin, code) is only validated and shown here;
 * the HTTPS handshake that trades the code for a token arrives with the API client (7b).
 */
class PairingActivity : Activity() {

    private lateinit var estado: TextView
    private lateinit var pegado: View

    /** Last valid QR; consumed by the handshake in 7b. */
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
        } catch (e: QrInvalidoException) {
            estado.text = getString(R.string.qr_invalido, e.message)
        }
    }
}
