package com.bythos.movil

import android.app.Activity
import android.content.Context
import android.text.InputType
import android.view.View
import android.widget.Button
import android.widget.EditText
import android.widget.LinearLayout
import com.google.android.gms.common.ConnectionResult
import com.google.android.gms.common.GoogleApiAvailability
import com.google.mlkit.vision.barcode.common.Barcode
import com.google.mlkit.vision.codescanner.GmsBarcodeScannerOptions
import com.google.mlkit.vision.codescanner.GmsBarcodeScanning

/**
 * Scans with the Play Services code scanner (no CAMERA permission needed). When
 * Play Services is missing, or its scanner module cannot be installed, [alFallar]
 * fires so the caller shows the manual-paste screen instead.
 */
object Escaner {

    fun escanear(activity: Activity, alTexto: (String) -> Unit, alFallar: () -> Unit) {
        if (!playServicesDisponible(activity)) {
            alFallar()
            return
        }
        val opciones = GmsBarcodeScannerOptions.Builder()
            .setBarcodeFormats(Barcode.FORMAT_QR_CODE)
            .build()
        // Cancel (user backs out) has its own callback and is deliberately not a failure.
        GmsBarcodeScanning.getClient(activity, opciones).startScan()
            .addOnSuccessListener { alTexto(it.rawValue.orEmpty()) }
            .addOnFailureListener { alFallar() }
    }

    private fun playServicesDisponible(contexto: Context): Boolean =
        GoogleApiAvailability.getInstance()
            .isGooglePlayServicesAvailable(contexto) == ConnectionResult.SUCCESS
}

/** Manual-paste screen for the bythos://pair?... text the PC shows as copyable. */
object PegadoManual {

    fun vista(activity: Activity, alTexto: (String) -> Unit): View {
        val campo = EditText(activity).apply {
            hint = activity.getString(R.string.pegar_pista)
            inputType = InputType.TYPE_CLASS_TEXT or InputType.TYPE_TEXT_VARIATION_URI
            minLines = 3
        }
        val boton = Button(activity).apply {
            text = activity.getString(R.string.usar_enlace)
            setOnClickListener { alTexto(campo.text.toString()) }
        }
        return LinearLayout(activity).apply {
            orientation = LinearLayout.VERTICAL
            addView(campo)
            addView(boton)
        }
    }
}
