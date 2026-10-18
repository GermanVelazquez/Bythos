package com.bythos.movil

import android.app.Activity
import android.os.Bundle

/** Clave (en las prefs "bythos") que recuerda que el tutorial ya se abrió solo una vez. */
const val CLAVE_TUTORIAL_VISTO = "tutorialVisto"

/** Pura y probable en JVM: el tutorial se abre solo únicamente en el primer uso desde el lanzador. */
fun debeAbrirTutorial(desdeLanzador: Boolean, emparejado: Boolean, visto: Boolean, recreada: Boolean): Boolean =
    desdeLanzador && !emparejado && !visto && !recreada

/** Cinco pasos de solo texto que explican cómo enviar archivos; "Entendido" y Atrás lo cierran. */
class TutorialActivity : Activity() {

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        val pasos = listOf(
            getString(R.string.tutorial_paso_1),
            getString(R.string.tutorial_paso_2, getString(R.string.escanear_qr)),
            getString(R.string.tutorial_paso_3),
            getString(R.string.tutorial_paso_4, getString(R.string.enviar_archivo), getString(R.string.app_name)),
            getString(R.string.tutorial_paso_5, getString(R.string.fase_listo)),
        )
        Estilo.pantalla(this).apply {
            addView(Estilo.titulo(context, getString(R.string.tutorial_titulo)))
            pasos.forEachIndexed { i, texto ->
                addView(Estilo.tarjeta(context, Estilo.panel(context, "${i + 1}"), Estilo.cuerpo(context, texto)))
            }
            addView(Estilo.botonPrimario(context, getString(R.string.entendido)) { finish() })
        }
    }
}
