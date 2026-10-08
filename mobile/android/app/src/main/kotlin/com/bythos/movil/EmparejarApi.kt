package com.bythos.movil

import org.json.JSONObject

data class SolicitudEmparejamiento(val id: String, val sas: String)

/** Pairing state as polled; [token] arrives exactly once, on the first poll after approval. */
data class EstadoEmparejamiento(val estado: String, val token: String?)

/** POST /v1/emparejar and GET /v1/emparejar/{solicitud} (no Bearer: the code is the credential). */
class EmparejarApi(private val api: ApiClient) {

    fun solicitar(codigo: String, nombre: String): SolicitudEmparejamiento {
        val cuerpo = JSONObject().put("codigo", codigo).put("nombre", nombre)
        val j = JSONObject(api.postJson("/v1/emparejar", cuerpo.toString()).cuerpo)
        return SolicitudEmparejamiento(j.getString("solicitud"), j.getString("sas"))
    }

    fun estado(solicitud: String): EstadoEmparejamiento {
        val j = JSONObject(api.get("/v1/emparejar/$solicitud").cuerpo)
        return EstadoEmparejamiento(j.getString("estado"), if (j.has("token")) j.getString("token") else null)
    }
}
