package com.bythos.movil

import org.junit.Assert.assertEquals
import org.junit.Assert.assertThrows
import org.junit.Test

class QrEmparejamientoTest {

    private val huella = "y2pkFNNj9CMzzX3tI989fn-YDN69RLCHbJgP4ohdP78"

    @Test
    fun parseaElTextoQueArmaElEscritorio() {
        // Same shape as lan.QRTexto: url.QueryEscape turns spaces into '+'.
        val texto = "bythos://pair?v=1&h=192.168.1.20&p=48080&k=$huella&c=abc_DEF-123&n=Mi+PC+%C3%B1and%C3%BA"
        val qr = parsearQr(texto)
        assertEquals(QrEmparejamiento("192.168.1.20", 48080, huella, "abc_DEF-123", "Mi PC ñandú"), qr)
    }

    @Test
    fun ignoraEspaciosAlrededorYElOrdenDeLosParametros() {
        val qr = parsearQr("  bythos://pair?n=PC&c=x&k=$huella&p=1&h=10.0.0.5&v=1\n")
        assertEquals("10.0.0.5", qr.host)
        assertEquals(1, qr.puerto)
    }

    @Test
    fun nombreVisibleUsaElGuardadoOElPorDefecto() {
        assertEquals("tu PC", nombreVisiblePC(null, "tu PC"))
        assertEquals("tu PC", nombreVisiblePC("  ", "tu PC"))
        assertEquals("ESCRITORIO", nombreVisiblePC("ESCRITORIO", "tu PC"))
    }

    @Test
    fun nombreEsOpcional() {
        assertEquals("", parsearQr("bythos://pair?v=1&h=h&p=80&k=$huella&c=x").nombrePC)
    }

    @Test
    fun rechazaLoQueNoEsUnEnlaceDeBythos() {
        listOf(
            "",
            "https://pair?v=1&h=h&p=80&k=$huella&c=x",
            "bythos://otro?v=1&h=h&p=80&k=$huella&c=x",
            "bythos://pair",
            "no es una url",
        ).forEach { assertThrows(it, QrInvalidoException::class.java) { parsearQr(it) } }
    }

    @Test
    fun rechazaParametrosInvalidos() {
        val base = "bythos://pair?v=1&h=h&p=80&k=$huella&c=x"
        listOf(
            base.replace("v=1", "v=2"),
            base.replace("h=h&", ""),
            base.replace("p=80", "p=0"),
            base.replace("p=80", "p=70000"),
            base.replace("p=80", "p=abc"),
            base.replace(huella, huella.dropLast(1)),
            base.replace(huella, huella.dropLast(1) + "+"),
            base.replace("&c=x", ""),
            base.replace("&c=x", "&c="),
        ).forEach { assertThrows(it, QrInvalidoException::class.java) { parsearQr(it) } }
    }
}
