package com.bythos.movil

import android.app.Activity
import android.content.pm.PackageManager
import android.os.Build

/** Shared by every screen that talks to the PC: local-network permission, failure text, paired client. */
abstract class ActividadRedLocal : Activity() {

    protected fun permisoRedLocal() =
        Build.VERSION.SDK_INT < SDK_RED_LOCAL || checkSelfPermission(PERMISO_RED_LOCAL) == PackageManager.PERMISSION_GRANTED

    private var siguiente: (() -> Unit)? = null

    /** Asks for ACCESS_LOCAL_NETWORK where the OS enforces it; a denial still proceeds so the timeout is classified. */
    protected fun conPermisoRedLocal(accion: () -> Unit) {
        if (permisoRedLocal()) return accion()
        siguiente = accion
        requestPermissions(arrayOf(PERMISO_RED_LOCAL), PEDIDO_RED_LOCAL)
    }

    override fun onRequestPermissionsResult(requestCode: Int, permissions: Array<out String>, grantResults: IntArray) {
        super.onRequestPermissionsResult(requestCode, permissions, grantResults)
        if (requestCode == PEDIDO_RED_LOCAL) siguiente?.invoke()
        siguiente = null
    }

    protected fun textoFallo(e: Exception): String = when (clasificarFallo(e, Build.VERSION.SDK_INT, permisoRedLocal())) {
        Fallo.PIN_CAMBIO -> getString(R.string.fallo_pin)
        Fallo.PERMISO_RED_LOCAL -> getString(R.string.fallo_red_local)
        Fallo.NO_EMPAREJADO -> getString(R.string.fallo_no_emparejado)
        Fallo.INALCANZABLE -> (e as? ErrorApi)?.let { getString(R.string.fallo_pc, it.message) }
            ?: getString(R.string.fallo_inalcanzable)
    }

    companion object {
        const val PERMISO_RED_LOCAL = "android.permission.ACCESS_LOCAL_NETWORK"
        private const val PEDIDO_RED_LOCAL = 1
    }
}

/** Client for the stored pairing (target + pin from prefs, token from the Keystore); null when not paired. */
fun clienteEmparejado(contexto: android.content.Context): ApiClient? {
    val prefs = contexto.getSharedPreferences("bythos", android.content.Context.MODE_PRIVATE)
    val host = prefs.getString("host", null) ?: return null
    val huella = prefs.getString("huella", null) ?: return null
    val token = KeystoreTokenStore.deContexto(prefs).leer() ?: return null
    return ApiClient(host, prefs.getInt("puerto", 48080), huella, token)
}
