package com.bythos.movil

import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

/** El tutorial solo se abre solo desde el lanzador, sin emparejar, sin haberlo visto y sin recreación. */
class TutorialDecisionTest {

    @Test
    fun soloLaCombinacionDePrimerUsoAbreElTutorial() {
        val valores = listOf(false, true)
        var abiertas = 0
        for (desdeLanzador in valores) for (emparejado in valores) for (visto in valores) for (recreada in valores) {
            val esperado = desdeLanzador && !emparejado && !visto && !recreada
            assertEquals(
                "lanzador=$desdeLanzador emparejado=$emparejado visto=$visto recreada=$recreada",
                esperado,
                debeAbrirTutorial(desdeLanzador, emparejado, visto, recreada),
            )
            if (esperado) abiertas++
        }
        assertEquals(1, abiertas)
    }

    @Test
    fun primerUsoDesdeElLanzadorAbre() {
        assertTrue(debeAbrirTutorial(desdeLanzador = true, emparejado = false, visto = false, recreada = false))
    }

    @Test
    fun lanzarDesdeCompartirNuncaAbre() {
        for (emparejado in listOf(false, true)) for (visto in listOf(false, true)) for (recreada in listOf(false, true)) {
            assertEquals(false, debeAbrirTutorial(false, emparejado, visto, recreada))
        }
    }
}
