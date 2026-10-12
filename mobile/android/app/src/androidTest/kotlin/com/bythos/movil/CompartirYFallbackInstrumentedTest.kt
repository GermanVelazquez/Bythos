package com.bythos.movil

import android.content.Intent
import android.view.View
import android.view.ViewGroup
import android.widget.Button
import android.widget.EditText
import androidx.test.core.app.ActivityScenario
import androidx.test.core.app.ApplicationProvider
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith

/** Humo en dispositivo: ruteo del compartir y el camino de pegado manual cuando no hay escáner. */
@RunWith(AndroidJUnit4::class)
class CompartirYFallbackInstrumentedTest {

    /** El escenario abre el lanzador (MAIN): sin esto el tutorial se abriría solo y taparía la pantalla. */
    @Before
    fun tutorialYaVisto() {
        ApplicationProvider.getApplicationContext<android.content.Context>()
            .getSharedPreferences("bythos", android.content.Context.MODE_PRIVATE)
            .edit().putBoolean(CLAVE_TUTORIAL_VISTO, true).commit()
    }

    @Test
    fun accionSendResuelveAShareReceiverActivity() {
        val contexto = ApplicationProvider.getApplicationContext<android.content.Context>()
        val intent = Intent(Intent.ACTION_SEND).apply {
            type = "text/plain"
            setPackage(contexto.packageName)
        }
        val resuelto = contexto.packageManager.queryIntentActivities(intent, 0)
        assertTrue(resuelto.any { it.activityInfo.name == ShareReceiverActivity::class.java.name })
    }

    @Test
    fun sinEscanerElPegadoManualQuedaDisponibleYRechazaTextoInvalido() {
        ActivityScenario.launch(PairingActivity::class.java).use { escenario ->
            escenario.onActivity { actividad ->
                // Mismo camino que alFallar del escáner: mostrar el pegado y entregar el texto.
                val raiz = actividad.findViewById<ViewGroup>(android.R.id.content)
                val pegar = hallar<Button>(raiz) { it.text == actividad.getString(R.string.pegar_enlace) }
                assertNotNull(pegar)
                pegar!!.performClick()

                val campo = hallar<EditText>(raiz) { true }
                assertNotNull(campo)
                assertEquals(View.VISIBLE, (campo!!.parent as View).visibility)

                campo.setText("esto no es un QR")
                hallar<Button>(raiz) { it.text == actividad.getString(R.string.usar_enlace) }!!.performClick()
                assertNull(actividad.qr)
            }
        }
        InstrumentationRegistry.getInstrumentation().waitForIdleSync()
    }

    @Suppress("UNCHECKED_CAST")
    private inline fun <reified T : View> hallar(vista: View, crossinline crit: (T) -> Boolean): T? =
        buscar(vista) { it is T && crit(it) } as T?

    private fun buscar(vista: View, crit: (View) -> Boolean): View? {
        if (crit(vista)) return vista
        if (vista is ViewGroup) {
            for (i in 0 until vista.childCount) buscar(vista.getChildAt(i), crit)?.let { return it }
        }
        return null
    }
}
