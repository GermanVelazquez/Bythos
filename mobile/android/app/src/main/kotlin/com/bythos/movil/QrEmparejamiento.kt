package com.bythos.movil

import java.net.URI
import java.net.URLDecoder

/** Pairing data carried by the QR: bythos://pair?v=1&h&p&k&c&n (see desktop/lan/qr.go). */
data class QrEmparejamiento(
    val host: String,
    val puerto: Int,
    /** SHA-256 of the server SubjectPublicKeyInfo, base64url without padding (43 chars). */
    val huella: String,
    val codigo: String,
    val nombrePC: String,
)

class QrInvalidoException(message: String) : Exception(message)

private val HUELLA_REGEX = Regex("^[A-Za-z0-9_-]{43}$")

/**
 * Parses the pairing text. Pure JVM on purpose (java.net.URI, no android.net.Uri)
 * so it runs in plain unit tests. Parameters are read by key, not by position.
 */
fun parsearQr(texto: String): QrEmparejamiento {
    val uri = try {
        URI(texto.trim())
    } catch (e: Exception) {
        throw QrInvalidoException("formato desconocido")
    }
    if (uri.scheme != "bythos" || uri.authority != "pair") {
        throw QrInvalidoException("no es un enlace de Bythos")
    }
    val query = uri.rawQuery ?: throw QrInvalidoException("faltan parámetros")
    val params = HashMap<String, String>()
    for (par in query.split('&')) {
        val i = par.indexOf('=')
        if (i <= 0) continue
        params[par.substring(0, i)] = URLDecoder.decode(par.substring(i + 1), "UTF-8")
    }
    if (params["v"] != "1") throw QrInvalidoException("versión no soportada")
    val host = params["h"]?.takeIf { it.isNotEmpty() } ?: throw QrInvalidoException("falta la dirección")
    val puerto = params["p"]?.toIntOrNull()?.takeIf { it in 1..65535 }
        ?: throw QrInvalidoException("puerto inválido")
    val huella = params["k"]?.takeIf { HUELLA_REGEX.matches(it) }
        ?: throw QrInvalidoException("huella inválida")
    val codigo = params["c"]?.takeIf { it.isNotEmpty() } ?: throw QrInvalidoException("falta el código")
    return QrEmparejamiento(host, puerto, huella, codigo, params["n"].orEmpty())
}
