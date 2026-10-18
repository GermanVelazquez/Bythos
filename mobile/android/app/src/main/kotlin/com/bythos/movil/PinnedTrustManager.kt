package com.bythos.movil

import java.security.MessageDigest
import java.security.cert.CertificateException
import java.security.cert.X509Certificate
import java.util.Base64
import javax.net.ssl.X509TrustManager

/** The server key does not match the pin from the QR. Extends CertificateException so JSSE surfaces it as the handshake cause. */
class PinMismatchException(message: String) : CertificateException(message)

/** SHA-256 of the SubjectPublicKeyInfo (X509Certificate.publicKey.encoded), base64url without padding. */
fun huellaSpki(cert: X509Certificate): String {
    val suma = MessageDigest.getInstance("SHA-256").digest(cert.publicKey.encoded)
    return Base64.getUrlEncoder().withoutPadding().encodeToString(suma)
}

/**
 * Trust is the pin, not a CA: the PC certificate is self-signed and reissued per
 * session, but its key (and so its SPKI hash) is stable.
 */
class PinnedTrustManager(private val huellaEsperada: String) : X509TrustManager {

    override fun checkServerTrusted(chain: Array<out X509Certificate>?, authType: String?) {
        val hoja = chain?.firstOrNull() ?: throw PinMismatchException("cadena vacía")
        val real = huellaSpki(hoja).toByteArray(Charsets.US_ASCII)
        // MessageDigest.isEqual is constant-time on the content for equal lengths.
        if (!MessageDigest.isEqual(real, huellaEsperada.toByteArray(Charsets.US_ASCII))) {
            throw PinMismatchException("la huella del servidor no coincide con la del QR")
        }
    }

    override fun checkClientTrusted(chain: Array<out X509Certificate>?, authType: String?) {
        throw PinMismatchException("este cliente no autentica con certificado")
    }

    override fun getAcceptedIssuers(): Array<X509Certificate> = emptyArray()
}
