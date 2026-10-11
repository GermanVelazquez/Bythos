package com.bythos.movil

import org.junit.Assert.assertEquals
import org.junit.Test
import java.io.IOException
import java.net.ConnectException
import java.net.SocketTimeoutException
import javax.net.ssl.SSLHandshakeException

class FailureClassifierTest {

    private data class Caso(val ex: Throwable, val sdk: Int, val permiso: Boolean, val esperado: Fallo)

    @Test
    fun tablaDeClasificacion() {
        val pin = SSLHandshakeException("x").apply { initCause(PinMismatchException("huella")) }
        val casos = listOf(
            Caso(pin, 34, true, Fallo.PIN_CAMBIO),
            Caso(pin, 37, false, Fallo.PIN_CAMBIO), // the pin wins over the permission hint
            Caso(ErrorApi(401, "token_invalido", "x"), 34, true, Fallo.NO_EMPAREJADO),
            Caso(SocketTimeoutException(), 37, false, Fallo.PERMISO_RED_LOCAL),
            Caso(ConnectException(), 37, false, Fallo.PERMISO_RED_LOCAL),
            Caso(SocketTimeoutException(), 37, true, Fallo.INALCANZABLE),
            Caso(SocketTimeoutException(), 34, false, Fallo.INALCANZABLE), // OS does not enforce it yet
            Caso(IOException("reset"), 37, false, Fallo.INALCANZABLE),
            Caso(ErrorApi(500, "error_interno", "x"), 37, false, Fallo.INALCANZABLE),
        )
        for (c in casos) assertEquals(c.toString(), c.esperado, clasificarFallo(c.ex, c.sdk, c.permiso))
    }
}
