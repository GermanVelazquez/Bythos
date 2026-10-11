package com.bythos.movil

import org.junit.Assert.assertEquals
import org.junit.Assert.assertThrows
import org.junit.Test
import java.security.cert.CertificateFactory
import java.security.cert.X509Certificate

/**
 * The fixture cert is the shared golden: spki.txt holds the value computed with
 * the same SHA-256(SubjectPublicKeyInfo) base64url rule as desktop/lan.FingerprintSPKI
 * (cross-checked against Go's x509 RawSubjectPublicKeyInfo).
 */
class PinnedTrustManagerTest {

    private fun recurso(nombre: String) =
        javaClass.getResourceAsStream("/fixture/$nombre")!!.use { it.readBytes() }

    private val cert: X509Certificate = CertificateFactory.getInstance("X.509")
        .generateCertificate(recurso("cert.pem").inputStream()) as X509Certificate

    private val huellaDorada = String(recurso("spki.txt")).trim()

    @Test
    fun laHuellaCoincideConElGoldenDelEscritorio() {
        assertEquals(43, huellaDorada.length)
        assertEquals(huellaDorada, huellaSpki(cert))
    }

    @Test
    fun aceptaLaCadenaConLaHuellaFijada() {
        PinnedTrustManager(huellaDorada).checkServerTrusted(arrayOf(cert), "ECDHE_ECDSA")
    }

    @Test
    fun rechazaOtraHuella() {
        val otra = huellaDorada.dropLast(1) + if (huellaDorada.last() == 'A') 'B' else 'A'
        assertThrows(PinMismatchException::class.java) {
            PinnedTrustManager(otra).checkServerTrusted(arrayOf(cert), "ECDHE_ECDSA")
        }
    }

    @Test
    fun rechazaCadenaVaciaONula() {
        val tm = PinnedTrustManager(huellaDorada)
        assertThrows(PinMismatchException::class.java) { tm.checkServerTrusted(emptyArray(), "x") }
        assertThrows(PinMismatchException::class.java) { tm.checkServerTrusted(null, "x") }
    }

    @Test
    fun nuncaConfiaEnClientes() {
        assertThrows(PinMismatchException::class.java) {
            PinnedTrustManager(huellaDorada).checkClientTrusted(arrayOf(cert), "RSA")
        }
    }

    @Test
    fun noAnunciaEmisores() {
        assertEquals(0, PinnedTrustManager(huellaDorada).acceptedIssuers.size)
    }
}
