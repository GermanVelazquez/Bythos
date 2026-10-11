package com.bythos.movil

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotEquals
import org.junit.Assert.assertNull
import org.junit.Test
import java.util.Base64
import javax.crypto.KeyGenerator
import javax.crypto.SecretKey

/** The Android Keystore is swapped for a software AES key; the store and cipher logic is the same code. */
class KeystoreTokenStoreTest {

    private class Memoria : ClaveValor {
        val datos = HashMap<String, String>()
        override fun leer(clave: String) = datos[clave]
        override fun escribir(clave: String, valor: String) { datos[clave] = valor }
        override fun borrar(clave: String) { datos.remove(clave) }
    }

    private fun clave(): SecretKey = KeyGenerator.getInstance("AES").apply { init(256) }.generateKey()

    private fun tienda(mem: Memoria, k: SecretKey = clave()) =
        KeystoreTokenStore(mem, AesGcmCipher { k })

    @Test
    fun guardaYLeeElToken() {
        val t = tienda(Memoria())
        t.guardar("tok_ñ-123")
        assertEquals("tok_ñ-123", t.leer())
    }

    @Test
    fun noGuardaElTokenEnClaro() {
        val mem = Memoria()
        tienda(mem).guardar("secreto-visible")
        assertFalse(mem.datos.values.any { it.contains("secreto-visible") })
        assertFalse(String(Base64.getDecoder().decode(mem.datos.values.single())).contains("secreto-visible"))
    }

    @Test
    fun cadaCifradoUsaUnIvDistinto() {
        val mem = Memoria()
        val t = tienda(mem)
        t.guardar("igual")
        val a = mem.datos.values.single()
        t.guardar("igual")
        assertNotEquals(a, mem.datos.values.single())
    }

    @Test
    fun sinTokenDevuelveNull() {
        assertNull(tienda(Memoria()).leer())
    }

    @Test
    fun claveInvalidadaDevuelveNullYLimpiaElBlob() {
        val mem = Memoria()
        tienda(mem).guardar("tok")
        assertNull(tienda(mem).leer()) // different key, as after Keystore invalidation
        assertEquals(0, mem.datos.size)
    }

    @Test
    fun blobAlteradoOCorruptoDevuelveNull() {
        val mem = Memoria()
        val k = clave()
        tienda(mem, k).guardar("tok")
        val blob = Base64.getDecoder().decode(mem.datos.values.single())
        blob[blob.size - 1] = (blob.last().toInt() xor 1).toByte()
        mem.datos.keys.single().let { mem.datos[it] = Base64.getEncoder().encodeToString(blob) }
        assertNull(tienda(mem, k).leer())

        mem.datos["token_cifrado"] = "no-es-base64!!"
        assertNull(tienda(mem, k).leer())
    }

    @Test
    fun borrarEliminaElToken() {
        val mem = Memoria()
        val t = tienda(mem)
        t.guardar("tok")
        t.borrar()
        assertNull(t.leer())
    }
}
