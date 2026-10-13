package com.bythos.movil

import org.junit.Assert.assertArrayEquals
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertThrows
import org.junit.Test
import java.io.ByteArrayInputStream
import java.io.FileNotFoundException

private class RepoMemoria(var fila: Envio) : EnviosRepo {
    override fun obtener(id: Long) = fila
    override fun guardar(envio: Envio) = envio.also { fila = it }
}

/** In-memory PC: keeps the received bytes and can inject the wire errors of the protocol. */
private class ServidorFalso(var recibido: ByteArray = ByteArray(0)) : Servidor {
    val ofsets = mutableListOf<Long>() // offsets of every parte call, to prove nothing is re-sent
    val errores = ArrayDeque<ErrorApi>()
    var remoto = "recibiendo"

    override fun crear(e: Envio) = SubidaCreada("s1", 0)
    override fun estado(id: String) = EstadoRemoto(remoto, recibido.size.toLong(), "")
    override fun enlace(carpetaId: Long, url: String) = Unit

    override fun parte(id: String, offset: Long, sha256: String, datos: ByteArray, progreso: (Long) -> Unit): Long {
        ofsets += offset
        // Bytes go out one by one before the PC answers, as with a real rejected chunk.
        for (n in 1..datos.size) progreso(n.toLong())
        errores.removeFirstOrNull()?.let { throw it }
        assertEquals(sha256Hex(datos), sha256)
        recibido += datos
        if (recibido.size >= TOTAL) remoto = "listo"
        return recibido.size.toLong()
    }

    companion object { const val TOTAL = 10 }
}

class SubidorTest {
    private val contenido = ByteArray(ServidorFalso.TOTAL) { it.toByte() }
    private fun envio(estado: EstadoEnvio = EstadoEnvio.PENDIENTE, subida: String? = null, offset: Long = 0) = Envio(
        id = 1, tipo = Envio.TIPO_ARCHIVO, uri = "content://x", nombre = "a.bin", tamano = contenido.size.toLong(),
        carpetaId = 7, clienteId = "c1", subidaId = subida, offset = offset, estado = estado,
    )

    private fun subidor(
        srv: Servidor, repo: RepoMemoria, cancelado: () -> Boolean = { false },
        fuente: Fuente = Fuente { ByteArrayInputStream(contenido) },
        avance: (Fase, Long, Long) -> Unit = { _, _, _ -> },
    ) = Subidor(srv, repo, fuente, cancelado, pausa = {}, tamanoParte = 4, avance = avance)

    private class Avance(val fase: Fase, val hechos: Long)

    @Test
    fun transicionesDelBuzon() {
        assertEquals(EstadoEnvio.SUBIENDO, transicion(EstadoEnvio.PENDIENTE, EventoEnvio.INICIAR))
        assertEquals(EstadoEnvio.PENDIENTE, transicion(EstadoEnvio.SUBIENDO, EventoEnvio.DETENER))
        assertEquals(EstadoEnvio.LISTO, transicion(EstadoEnvio.SUBIENDO, EventoEnvio.COMPLETAR))
        assertEquals(EstadoEnvio.SIN_EMPAREJAR, transicion(EstadoEnvio.SUBIENDO, EventoEnvio.DESEMPAREJAR))
        assertEquals(EstadoEnvio.PENDIENTE, transicion(EstadoEnvio.ERROR, EventoEnvio.REINTENTAR))
        assertNull(transicion(EstadoEnvio.LISTO, EventoEnvio.INICIAR))
        assertNull(transicion(EstadoEnvio.PENDIENTE, EventoEnvio.COMPLETAR))
        assertThrows(IllegalStateException::class.java) { envio().con(EventoEnvio.COMPLETAR) }
    }

    @Test
    fun subeTodoDesdeCero() {
        val srv = ServidorFalso()
        val repo = RepoMemoria(envio())
        assertEquals(Resultado.LISTO, subidor(srv, repo).ejecutar(1))
        assertArrayEquals(contenido, srv.recibido)
        assertEquals(EstadoEnvio.LISTO, repo.fila.estado)
        assertEquals(sha256Hex(contenido), repo.fila.sha256)
    }

    @Test
    fun reanudaDesdeElOffsetDeLaPcSinReenviar() {
        // The process died mid-upload: the row says 0 but the PC already holds 4 bytes.
        val srv = ServidorFalso(contenido.copyOf(4))
        val repo = RepoMemoria(envio(EstadoEnvio.SUBIENDO, subida = "s1", offset = 0))
        assertEquals(Resultado.LISTO, subidor(srv, repo).ejecutar(1))
        assertEquals(listOf(4L, 8L), srv.ofsets)
        assertArrayEquals(contenido, srv.recibido)
    }

    @Test
    fun conflictoDeOffsetRetomaDesdeLoQueDiceLaPc() {
        val srv = ServidorFalso(contenido.copyOf(4))
        srv.errores += ErrorApi(409, "offset_invalido", "x", offset = 4)
        val repo = RepoMemoria(envio(EstadoEnvio.PENDIENTE, subida = "s1", offset = 0).copy(sha256 = "x"))
        assertEquals(Resultado.LISTO, subidor(srv, repo).ejecutar(1))
        assertEquals(listOf(4L, 4L, 8L), srv.ofsets)
        assertArrayEquals(contenido, srv.recibido)
    }

    @Test
    fun hashDeParteInvalidoReintentaLaMismaParte() {
        val srv = ServidorFalso()
        srv.errores += ErrorApi(422, "hash_parte_invalido", "x")
        val repo = RepoMemoria(envio())
        assertEquals(Resultado.LISTO, subidor(srv, repo).ejecutar(1))
        assertEquals(listOf(0L, 0L, 4L, 8L), srv.ofsets)
    }

    @Test
    fun token401MarcaSinEmparejar() {
        val srv = ServidorFalso()
        srv.errores += ErrorApi(401, "token_invalido", "x")
        val repo = RepoMemoria(envio())
        assertEquals(Resultado.FALLO, subidor(srv, repo).ejecutar(1))
        assertEquals(EstadoEnvio.SIN_EMPAREJAR, repo.fila.estado)
    }

    @Test
    fun fuenteBorradaACaminoFallaConErrorVisible() {
        var aperturas = 0
        val fuente = Fuente { if (++aperturas > 2) throw FileNotFoundException("gone") else ByteArrayInputStream(contenido) }
        val repo = RepoMemoria(envio())
        assertEquals(Resultado.FALLO, subidor(ServidorFalso(), repo, fuente = fuente).ejecutar(1))
        assertEquals(EstadoEnvio.ERROR, repo.fila.estado)
        assertEquals("fuente_inaccesible", repo.fila.error)
        assertEquals(4L, repo.fila.offset)
    }

    @Test
    fun alDetenerPersisteElOffsetYVuelveAPendiente() {
        var vueltas = 0
        val repo = RepoMemoria(envio())
        val r = subidor(ServidorFalso(), repo, cancelado = { ++vueltas > 1 }).ejecutar(1)
        assertEquals(Resultado.REINTENTAR, r)
        assertEquals(EstadoEnvio.PENDIENTE, repo.fila.estado)
        assertEquals(4L, repo.fila.offset)
    }

    @Test
    fun avanzaPorFasesYElEnvioTerminaEnElTamano() {
        val vistos = mutableListOf<Avance>()
        subidor(ServidorFalso(), RepoMemoria(envio()), avance = { f, h, _ -> vistos += Avance(f, h) }).ejecutar(1)
        assertEquals(listOf(Fase.PREPARANDO, Fase.ENVIANDO, Fase.VERIFICANDO), vistos.map { it.fase }.distinct())
        assertEquals(Fase.VERIFICANDO, vistos.last().fase)
        assertEquals(contenido.size.toLong(), vistos.last { it.fase == Fase.PREPARANDO }.hechos)
        val enviados = vistos.filter { it.fase == Fase.ENVIANDO }.map { it.hechos }
        assertEquals(enviados.sorted(), enviados)
        assertEquals(contenido.size.toLong(), enviados.last())
    }

    @Test
    fun reenviarUnaParteNoHaceRetrocederElAvance() {
        val enviados = mutableListOf<Long>()
        val srv = ServidorFalso()
        srv.errores += ErrorApi(422, "hash_parte_invalido", "x")
        subidor(srv, RepoMemoria(envio()), avance = { f, h, _ -> if (f == Fase.ENVIANDO) enviados += h }).ejecutar(1)
        assertEquals(enviados.sorted(), enviados)
        assertEquals(contenido.size.toLong(), enviados.last())
    }

    @Test
    fun alReanudarElEnvioArrancaEnElOffset() {
        val vistos = mutableListOf<Avance>()
        val srv = ServidorFalso(contenido.copyOf(4))
        val repo = RepoMemoria(envio(EstadoEnvio.SUBIENDO, subida = "s1", offset = 0))
        subidor(srv, repo, avance = { f, h, _ -> vistos += Avance(f, h) }).ejecutar(1)
        assertEquals(4L, vistos.first { it.fase == Fase.ENVIANDO }.hechos)
    }

    @Test
    fun unArchivoMasChicoQueUnaParteTambienAvanza() {
        val vistos = mutableListOf<Long>()
        val repo = RepoMemoria(envio())
        Subidor(ServidorFalso(), repo, { ByteArrayInputStream(contenido) }, pausa = {}, tamanoParte = 64,
            avance = { f, h, _ -> if (f == Fase.ENVIANDO) vistos += h }).ejecutar(1)
        assertEquals(contenido.size.toLong(), vistos.last())
        assertEquals(true, vistos.size >= 2 && vistos.first() < vistos.last())
    }

    @Test
    fun elLimitadorDescartaDentroDeLosMsYDejaPasarFaseYFinal() {
        var ahora = 0L
        val l = LimitadorAvance(250) { ahora }
        assertEquals(true, l.pasa(Fase.ENVIANDO, 1, 10))
        ahora = 100
        assertEquals(false, l.pasa(Fase.ENVIANDO, 2, 10))
        ahora = 300
        assertEquals(true, l.pasa(Fase.ENVIANDO, 3, 10))
        ahora = 310
        assertEquals(true, l.pasa(Fase.ENVIANDO, 10, 10)) // completion
        ahora = 320
        assertEquals(true, l.pasa(Fase.VERIFICANDO, 10, 10)) // phase change
    }
}
