package com.bythos.movil

import org.json.JSONArray

data class Carpeta(val id: Long, val nombre: String)

/** GET /v1/carpetas: the folders the phone can send into. */
class FoldersApi(private val api: ApiClient) {

    fun listar(): List<Carpeta> {
        val arr = JSONArray(api.get("/v1/carpetas").cuerpo)
        return List(arr.length()) { i -> arr.getJSONObject(i).let { Carpeta(it.getLong("id"), it.getString("nombre")) } }
    }
}
