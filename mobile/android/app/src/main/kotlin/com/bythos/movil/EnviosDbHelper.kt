package com.bythos.movil

import android.content.ContentValues
import android.content.Context
import android.database.Cursor
import android.database.sqlite.SQLiteDatabase
import android.database.sqlite.SQLiteOpenHelper

/** Thin SQLite wrapper for the `envios` outbox; the rules live in Envio.kt. */
class EnviosDbHelper(contexto: Context) : SQLiteOpenHelper(contexto, "envios.db", null, 1), EnviosRepo {

    override fun onCreate(db: SQLiteDatabase) {
        db.execSQL(
            "CREATE TABLE envios (id INTEGER PRIMARY KEY AUTOINCREMENT, tipo TEXT NOT NULL, uri TEXT NOT NULL, " +
                "nombre TEXT NOT NULL, tamano INTEGER NOT NULL, sha256 TEXT, carpeta_id INTEGER NOT NULL, " +
                "cliente_id TEXT NOT NULL UNIQUE, subida_id TEXT, offset INTEGER NOT NULL DEFAULT 0, " +
                "estado TEXT NOT NULL, error TEXT NOT NULL DEFAULT '')",
        )
    }

    override fun onUpgrade(db: SQLiteDatabase, antes: Int, ahora: Int) = Unit

    override fun obtener(id: Long): Envio? =
        readableDatabase.query("envios", null, "id = ?", arrayOf(id.toString()), null, null, null)
            .use { if (it.moveToFirst()) leer(it) else null }

    /** Inserts when id is 0, otherwise updates; returns the stored row. */
    override fun guardar(envio: Envio): Envio {
        val v = ContentValues().apply {
            put("tipo", envio.tipo); put("uri", envio.uri); put("nombre", envio.nombre)
            put("tamano", envio.tamano); put("sha256", envio.sha256); put("carpeta_id", envio.carpetaId)
            put("cliente_id", envio.clienteId); put("subida_id", envio.subidaId); put("offset", envio.offset)
            put("estado", envio.estado.name); put("error", envio.error)
        }
        if (envio.id == 0L) return envio.copy(id = writableDatabase.insertOrThrow("envios", null, v))
        writableDatabase.update("envios", v, "id = ?", arrayOf(envio.id.toString()))
        return envio
    }

    private fun leer(c: Cursor) = Envio(
        id = c.getLong(c.getColumnIndexOrThrow("id")),
        tipo = c.getString(c.getColumnIndexOrThrow("tipo")),
        uri = c.getString(c.getColumnIndexOrThrow("uri")),
        nombre = c.getString(c.getColumnIndexOrThrow("nombre")),
        tamano = c.getLong(c.getColumnIndexOrThrow("tamano")),
        sha256 = c.getString(c.getColumnIndexOrThrow("sha256")),
        carpetaId = c.getLong(c.getColumnIndexOrThrow("carpeta_id")),
        clienteId = c.getString(c.getColumnIndexOrThrow("cliente_id")),
        subidaId = c.getString(c.getColumnIndexOrThrow("subida_id")),
        offset = c.getLong(c.getColumnIndexOrThrow("offset")),
        estado = EstadoEnvio.valueOf(c.getString(c.getColumnIndexOrThrow("estado"))),
        error = c.getString(c.getColumnIndexOrThrow("error")),
    )
}
