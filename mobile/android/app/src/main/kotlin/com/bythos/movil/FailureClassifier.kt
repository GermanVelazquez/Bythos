package com.bythos.movil

import java.net.ConnectException
import java.net.SocketTimeoutException

enum class Fallo { PIN_CAMBIO, PERMISO_RED_LOCAL, INALCANZABLE, NO_EMPAREJADO }

/** First SDK level that enforces ACCESS_LOCAL_NETWORK (Android 17). */
const val SDK_RED_LOCAL = 37

/**
 * Maps a request failure to the hint the user needs. A denied local-network permission
 * is silent: the OS just drops the connection, so it shows up as a timeout/refusal.
 */
fun clasificarFallo(ex: Throwable, sdkInt: Int, permisoRedLocal: Boolean): Fallo {
    val cadena = generateSequence(ex) { it.cause }.take(10).toList()
    if (cadena.any { it is PinMismatchException }) return Fallo.PIN_CAMBIO
    if (cadena.any { it is ErrorApi && it.status == 401 }) return Fallo.NO_EMPAREJADO
    val sinRespuesta = cadena.any { it is SocketTimeoutException || it is ConnectException }
    if (sinRespuesta && sdkInt >= SDK_RED_LOCAL && !permisoRedLocal) return Fallo.PERMISO_RED_LOCAL
    return Fallo.INALCANZABLE
}
