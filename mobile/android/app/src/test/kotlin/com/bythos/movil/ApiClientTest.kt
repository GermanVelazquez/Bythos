package com.bythos.movil

import com.sun.net.httpserver.HttpExchange
import com.sun.net.httpserver.HttpsConfigurator
import com.sun.net.httpserver.HttpsServer
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertThrows
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import java.net.InetAddress
import java.net.InetSocketAddress
import java.security.KeyFactory
import java.security.KeyStore
import java.security.cert.CertificateFactory
import java.security.spec.PKCS8EncodedKeySpec
import java.util.Base64
import javax.net.ssl.KeyManagerFactory
import javax.net.ssl.SSLContext

/** Real TLS against an in-process HttpsServer on 127.0.0.1 that serves the fixture key/cert. */
class ApiClientTest {

    private lateinit var servidor: HttpsServer
    private val huella = String(recurso("spki.txt")).trim()
    private val vistas = mutableListOf<Vista>()

    class Vista(val metodo: String, val ruta: String, val cabeceras: Map<String, List<String>>, val cuerpo: ByteArray)

    @Before
    fun arrancar() {
        val cert = CertificateFactory.getInstance("X.509").generateCertificate(recurso("cert.pem").inputStream())
        val pem = String(recurso("clave.pem")).lines().filter { !it.startsWith("-----") }.joinToString("")
        val clave = KeyFactory.getInstance("EC").generatePrivate(PKCS8EncodedKeySpec(Base64.getDecoder().decode(pem)))
        val ks = KeyStore.getInstance("PKCS12").apply {
            load(null)
            setKeyEntry("k", clave, CharArray(0), arrayOf(cert))
        }
        val kmf = KeyManagerFactory.getInstance(KeyManagerFactory.getDefaultAlgorithm()).apply { init(ks, CharArray(0)) }
        val ctx = SSLContext.getInstance("TLS").apply { init(kmf.keyManagers, null, null) }
        servidor = HttpsServer.create(InetSocketAddress(InetAddress.getByName("127.0.0.1"), 0), 0)
        servidor.httpsConfigurator = HttpsConfigurator(ctx)
        servidor.createContext("/") { ex -> atender(ex) }
        servidor.start()
    }

    @After
    fun parar() = servidor.stop(0)

    private fun recurso(nombre: String) = javaClass.getResourceAsStream("/fixture/$nombre")!!.use { it.readBytes() }

    private fun responder(ex: HttpExchange, status: Int, json: String) {
        val b = json.toByteArray()
        ex.responseHeaders.add("Content-Type", "application/json")
        ex.sendResponseHeaders(status, b.size.toLong())
        ex.responseBody.use { it.write(b) }
    }

    private fun atender(ex: HttpExchange) {
        vistas += Vista(ex.requestMethod, ex.requestURI.path, ex.requestHeaders.toMap(), ex.requestBody.readBytes())
        when (ex.requestURI.path) {
            "/v1/carpetas" -> responder(ex, 200, """[{"id":1,"nombre":"Videos"},{"id":2,"nombre":"Docs"}]""")
            "/v1/links" -> responder(ex, 201, """{"recurso_id":9,"titulo":"Ejemplo"}""")
            "/v1/subidas/a/partes" -> responder(ex, 409, """{"error":"offset_invalido","mensaje":"x","offset":4096}""")
            "/v1/subidas/b/partes" -> responder(ex, 422, """{"error":"hash_parte_invalido","mensaje":"x"}""")
            "/v1/subidas/c/partes" -> responder(ex, 200, """{"offset":5}""")
            else -> responder(ex, 401, """{"error":"token_invalido","mensaje":"x"}""")
        }
    }

    private fun cliente(huella: String = this.huella, token: String? = "tok") =
        ApiClient("127.0.0.1", servidor.address.port, huella, token)

    @Test
    fun conLaHuellaCorrectaLeeCarpetasConBearer() {
        val carpetas = FoldersApi(cliente()).listar()
        assertEquals(listOf(Carpeta(1, "Videos"), Carpeta(2, "Docs")), carpetas)
        assertEquals(listOf("Bearer tok"), vistas.single().cabeceras["Authorization"])
    }

    @Test
    fun enviaUnLinkComoJson() {
        val r = LinksApi(cliente()).enviar(1, "https://ejemplo.com", "")
        assertEquals(LinkGuardado(9, "Ejemplo"), r)
        assertTrue(String(vistas.single().cuerpo).contains("\"carpeta_id\":1"))
    }

    @Test
    fun conOtraHuellaRechazaYSeClasificaComoPin() {
        val otra = huella.dropLast(1) + if (huella.last() == 'A') 'B' else 'A'
        val ex = assertThrows(Exception::class.java) { FoldersApi(cliente(otra)).listar() }
        assertEquals(Fallo.PIN_CAMBIO, clasificarFallo(ex, 34, true))
        assertTrue(vistas.isEmpty())
    }

    @Test
    fun elVerificadorSoloAceptaElHostEmparejado() {
        assertTrue(hostCoincide("192.168.1.5", "192.168.1.5"))
        assertFalse(hostCoincide("192.168.1.5", "192.168.1.6"))
        assertFalse(hostCoincide("192.168.1.5", "evil.example"))
    }

    @Test
    fun un409LlevaElOffsetParaRetomar() {
        val ex = assertThrows(ErrorApi::class.java) { cliente().postBytes("/v1/subidas/a/partes", byteArrayOf(1)) }
        assertEquals(409, ex.status)
        assertEquals("offset_invalido", ex.codigo)
        assertEquals(4096L, ex.offset)
    }

    @Test
    fun un422SeExponeParaReintentarLaParte() {
        val ex = assertThrows(ErrorApi::class.java) { cliente().postBytes("/v1/subidas/b/partes", byteArrayOf(1)) }
        assertEquals(422, ex.status)
        assertEquals("hash_parte_invalido", ex.codigo)
        assertNull(ex.offset)
    }

    @Test
    fun laParteViajaConLongitudFijaSinChunked() {
        val datos = ByteArray(100_000) { (it % 251).toByte() }
        val r = cliente().postStream("/v1/subidas/c/partes", datos.size.toLong(), mapOf("X-Offset" to "0")) { it.write(datos) }
        assertEquals(200, r.status)
        val v = vistas.single()
        assertEquals("POST", v.metodo)
        assertEquals(listOf("100000"), v.cabeceras["Content-length"])
        assertNull(v.cabeceras["Transfer-encoding"])
        assertTrue(datos.contentEquals(v.cuerpo))
    }

    @Test
    fun escribirEnTramosConservaLosBytesYReportaAcumulado() {
        val datos = ByteArray(150 * 1024) { it.toByte() }
        val salida = java.io.ByteArrayOutputStream()
        val vistos = mutableListOf<Long>()
        escribirEnTramos(salida, datos, { vistos += it })
        assertTrue(datos.contentEquals(salida.toByteArray()))
        assertEquals(listOf(64L * 1024, 128L * 1024, 150L * 1024), vistos)
    }

    @Test
    fun losTimeoutsPorDefectoNoCambian() {
        assertEquals(5_000, ApiClient.CONEXION_MS)
        assertEquals(60_000, ApiClient.LECTURA_MS)
    }

    @Test
    fun unTokenInvalidoSeClasificaComoNoEmparejado() {
        val ex = assertThrows(ErrorApi::class.java) { cliente().get("/v1/otra") }
        assertEquals(Fallo.NO_EMPAREJADO, clasificarFallo(ex, 34, true))
    }
}
