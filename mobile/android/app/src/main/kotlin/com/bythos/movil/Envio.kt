package com.bythos.movil

enum class EstadoEnvio { PENDIENTE, SUBIENDO, LISTO, ERROR, SIN_EMPAREJAR }

enum class EventoEnvio { INICIAR, DETENER, COMPLETAR, FALLAR, DESEMPAREJAR, REINTENTAR }

/** Outbox state machine; null means the event is not valid in that state. */
fun transicion(actual: EstadoEnvio, evento: EventoEnvio): EstadoEnvio? = when (actual) {
    EstadoEnvio.PENDIENTE -> when (evento) {
        EventoEnvio.INICIAR -> EstadoEnvio.SUBIENDO
        EventoEnvio.FALLAR -> EstadoEnvio.ERROR
        EventoEnvio.DESEMPAREJAR -> EstadoEnvio.SIN_EMPAREJAR
        else -> null
    }
    EstadoEnvio.SUBIENDO -> when (evento) {
        EventoEnvio.DETENER -> EstadoEnvio.PENDIENTE
        EventoEnvio.COMPLETAR -> EstadoEnvio.LISTO
        EventoEnvio.FALLAR -> EstadoEnvio.ERROR
        EventoEnvio.DESEMPAREJAR -> EstadoEnvio.SIN_EMPAREJAR
        else -> null
    }
    EstadoEnvio.ERROR, EstadoEnvio.SIN_EMPAREJAR ->
        if (evento == EventoEnvio.REINTENTAR) EstadoEnvio.PENDIENTE else null
    EstadoEnvio.LISTO -> null
}

/** One outbox row: a file (uri) or a link (uri holds the URL) bound for a PC folder. */
data class Envio(
    val id: Long = 0,
    val tipo: String, // TIPO_ARCHIVO | TIPO_LINK
    val uri: String,
    val nombre: String,
    val tamano: Long,
    val carpetaId: Long,
    val clienteId: String,
    val sha256: String? = null,
    val subidaId: String? = null,
    val offset: Long = 0,
    val estado: EstadoEnvio = EstadoEnvio.PENDIENTE,
    val error: String = "",
) {
    fun con(evento: EventoEnvio, error: String = ""): Envio {
        val nuevo = transicion(estado, evento) ?: throw IllegalStateException("$evento no vale en $estado")
        return copy(estado = nuevo, error = error)
    }

    companion object {
        const val TIPO_ARCHIVO = "archivo"
        const val TIPO_LINK = "link"
    }
}

/** Persistence seam for the outbox, so the upload loop runs against an in-memory fake on the JVM. */
interface EnviosRepo {
    fun obtener(id: Long): Envio?
    fun guardar(envio: Envio): Envio
}
