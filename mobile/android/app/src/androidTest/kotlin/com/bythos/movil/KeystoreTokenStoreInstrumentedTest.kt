package com.bythos.movil

import android.content.Context
import androidx.test.core.app.ApplicationProvider
import androidx.test.ext.junit.runners.AndroidJUnit4
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertNull
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith

/** Round-trip contra el Keystore real del dispositivo: el token solo se guarda cifrado. */
@RunWith(AndroidJUnit4::class)
class KeystoreTokenStoreInstrumentedTest {

    private val prefsNombre = "bythos_test_keystore"
    private lateinit var almacen: KeystoreTokenStore
    private lateinit var contexto: Context

    @Before
    fun preparar() {
        contexto = ApplicationProvider.getApplicationContext()
        val prefs = contexto.getSharedPreferences(prefsNombre, Context.MODE_PRIVATE)
        prefs.edit().clear().commit()
        almacen = KeystoreTokenStore(
            SharedPreferencesClaveValor(prefs),
            AesGcmCipher.androidKeystore(alias = "bythos_token_test"),
        )
    }

    @After
    fun limpiar() {
        almacen.borrar()
    }

    @Test
    fun guardarYLeerDevuelveElMismoToken() {
        almacen.guardar("token-secreto-123")
        assertEquals("token-secreto-123", almacen.leer())
    }

    @Test
    fun elTokenNoQuedaEnClaroEnDisco() {
        almacen.guardar("token-secreto-123")
        val prefs = contexto.getSharedPreferences(prefsNombre, Context.MODE_PRIVATE)
        val crudo = prefs.all.values.joinToString()
        assertFalse(crudo.contains("token-secreto-123"))
    }

    @Test
    fun borrarDejaElAlmacenVacio() {
        almacen.guardar("token-secreto-123")
        almacen.borrar()
        assertNull(almacen.leer())
    }
}
