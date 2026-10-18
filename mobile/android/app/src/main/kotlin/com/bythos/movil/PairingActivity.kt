package com.bythos.movil

import android.app.Activity
import android.content.pm.PackageManager
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
class PairingActivity : Activity() {

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
            conPermisoRedLocal { emparejar(leido) }
        } catch (e: QrInvalidoException) {
            estado.text = getString(R.string.qr_invalido, e.message)
        }
    }

    private fun permisoRedLocal() =
        Build.VERSION.SDK_INT < SDK_RED_LOCAL || checkSelfPermission(PERMISO_RED_LOCAL) == PackageManager.PERMISSION_GRANTED

    private var siguiente: (() -> Unit)? = null

    /** Asks for ACCESS_LOCAL_NETWORK where the OS enforces it; a denial still proceeds so the timeout is classified. */
    private fun conPermisoRedLocal(accion: () -> Unit) {
        if (permisoRedLocal()) return accion()
        siguiente = accion
        requestPermissions(arrayOf(PERMISO_RED_LOCAL), PEDIDO_RED_LOCAL)
    }

    override fun onRequestPermissionsResult(requestCode: Int, permissions: Array<out String>, grantResults: IntArray) {
        super.onRequestPermissionsResult(requestCode, permissions, grantResults)
        if (requestCode == PEDIDO_RED_LOCAL) siguiente?.invoke()
        siguiente = null
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

    private fun textoFallo(e: Exception): String = when (clasificarFallo(e, Build.VERSION.SDK_INT, permisoRedLocal())) {
        Fallo.PIN_CAMBIO -> getString(R.string.fallo_pin)
        Fallo.PERMISO_RED_LOCAL -> getString(R.string.fallo_red_local)
        Fallo.NO_EMPAREJADO -> getString(R.string.fallo_no_emparejado)
        Fallo.INALCANZABLE -> (e as? ErrorApi)?.let { getString(R.string.fallo_pc, it.message) }
            ?: getString(R.string.fallo_inalcanzable)
    }

    private fun mostrar(texto: String) = runOnUiThread { estado.text = texto }

    private companion object {
        const val PERMISO_RED_LOCAL = "android.permission.ACCESS_LOCAL_NETWORK"
        const val PEDIDO_RED_LOCAL = 1
        const val SONDEO_MS = 2_000L
        const val ESPERA_MAX_MS = 10 * 60_000L
    }
}
