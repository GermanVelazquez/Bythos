package com.bythos.movil

import org.json.JSONObject
import java.io.IOException
import java.io.OutputStream
import java.net.URL
import javax.net.ssl.HttpsURLConnection
import javax.net.ssl.SSLContext

/** A wire error from the PC: {"error","mensaje"} plus the optional "offset" of a 409 offset_invalido. */
class ErrorApi(val status: Int, val codigo: String, mensaje: String, val offset: Long? = null) : IOException(mensaje)

class Respuesta(val status: Int, val cuerpo: String)

/** The connection target must be the paired host; trust itself comes from the pin. */
fun hostCoincide(esperado: String, recibido: String) = esperado.equals(recibido, ignoreCase = true)

/**
 * One HttpsURLConnection per request, trusting only the pinned SPKI. GET and POST only
 * (the PC protocol has no other verbs). Blocking: call it off the main thread.
 */
class ApiClient(
    private val host: String,
    private val puerto: Int,
    huella: String,
    private val token: String? = null,
) {
    private val fabrica = SSLContext.getInstance("TLS")
        .apply { init(null, arrayOf(PinnedTrustManager(huella)), null) }
        .socketFactory

    fun get(ruta: String): Respuesta = enviar("GET", ruta, null, emptyMap(), null)

    fun postJson(ruta: String, json: String): Respuesta =
        postBytes(ruta, json.toByteArray(Charsets.UTF_8), mapOf("Content-Type" to "application/json"))

    fun postBytes(ruta: String, cuerpo: ByteArray, cabeceras: Map<String, String> = emptyMap()): Respuesta =
        postStream(ruta, cuerpo.size.toLong(), cabeceras) { it.write(cuerpo) }

    /** Streams exactly [largo] bytes (fixed-length mode: no buffering, no chunked encoding). */
    fun postStream(ruta: String, largo: Long, cabeceras: Map<String, String>, escribir: (OutputStream) -> Unit): Respuesta =
        enviar("POST", ruta, largo, cabeceras, escribir)

    private fun enviar(
        metodo: String,
        ruta: String,
        largo: Long?,
        cabeceras: Map<String, String>,
        escribir: ((OutputStream) -> Unit)?,
    ): Respuesta {
        val con = URL("https", host, puerto, ruta).openConnection() as HttpsURLConnection
        try {
            con.sslSocketFactory = fabrica
            con.hostnameVerifier = javax.net.ssl.HostnameVerifier { h, _ -> hostCoincide(host, h) }
            con.connectTimeout = CONEXION_MS
            con.readTimeout = LECTURA_MS
            con.instanceFollowRedirects = false
            con.requestMethod = metodo
            token?.let { con.setRequestProperty("Authorization", "Bearer $it") }
            cabeceras.forEach { (k, v) -> con.setRequestProperty(k, v) }
            if (escribir != null && largo != null) {
                con.doOutput = true
                con.setFixedLengthStreamingMode(largo)
                con.outputStream.use(escribir)
            }
            val status = con.responseCode
            if (status < 400) {
                return Respuesta(status, con.inputStream.use { String(it.readBytes(), Charsets.UTF_8) })
            }
            val texto = con.errorStream?.use { String(it.readBytes(), Charsets.UTF_8) }.orEmpty()
            throw errorDe(status, texto)
        } finally {
            con.disconnect()
        }
    }

    private fun errorDe(status: Int, texto: String): ErrorApi = try {
        val j = JSONObject(texto)
        ErrorApi(status, j.optString("error"), j.optString("mensaje"), if (j.has("offset")) j.getLong("offset") else null)
    } catch (e: Exception) {
        ErrorApi(status, "", "HTTP $status")
    }

    companion object {
        const val CONEXION_MS = 5_000
        const val LECTURA_MS = 60_000
    }
}
