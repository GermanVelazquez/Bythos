package com.bythos.movil

import org.json.JSONObject

data class LinkGuardado(val recursoId: Long, val titulo: String)

/** POST /v1/links: the PC validates the folder, fetches metadata and stores the link. */
class LinksApi(private val api: ApiClient) {

    fun enviar(carpetaId: Long, url: String, titulo: String = ""): LinkGuardado {
        val cuerpo = JSONObject().put("carpeta_id", carpetaId).put("url", url).put("titulo", titulo)
        val j = JSONObject(api.postJson("/v1/links", cuerpo.toString()).cuerpo)
        return LinkGuardado(j.getLong("recurso_id"), j.optString("titulo"))
    }
}
