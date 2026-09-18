package api

// agenda.go — La AGENDA por HTTP. El calendario de la vista Todos bebe de aquí:
// - GET /api/agenda?desde=YYYY-MM-DD&hasta=YYYY-MM-DD (notas del rango)
// - POST /api/agenda {fecha, texto, hora_inicio?, hora_fin?, carpeta_id?}
// - DELETE /api/agenda/{id}
// - GET /api/actividad?desde=&hasta= (puntos de historia: creaciones por día)
//
// ¿Por qué archivo propio y no en server.go?
// Misma regla que export.go/avance.go: server.go = el menú,
// cada recurso = su archivo. El menú no cocina.

import (
	"encoding/json"
	"net/http"
	"strconv"

	"bythos-desktop/db"
)

// listarAgenda atiende GET /api/agenda?desde=&hasta=.
// Sin rango trae todo (igual que db.ListarAgenda con filtros vacíos).
func (s *Servidor) listarAgenda(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	items, err := db.ListarAgenda(s.Base, q.Get("desde"), q.Get("hasta"))
	if err != nil {
		responderError(w, 500, "No se pudo leer la agenda")
		return
	}
	responder(w, items)
}

// crearAgenda atiende POST /api/agenda.
// Las reglas viven en db.CrearAgenda (la BD es la última defensa);
// aquí solo traducimos su error en español a un 400 tal cual.
func (s *Servidor) crearAgenda(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Fecha      string `json:"fecha"`
		Texto      string `json:"texto"`
		HoraInicio string `json:"hora_inicio"`
		HoraFin    string `json:"hora_fin"`
		CarpetaID  *int64 `json:"carpeta_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		responderError(w, 400, "JSON inválido. Manda {\"fecha\":\"YYYY-MM-DD\",\"texto\":\"...\"}")
		return
	}
	a, err := db.CrearAgenda(s.Base, body.Fecha, body.HoraInicio, body.HoraFin, body.Texto, body.CarpetaID)
	if err != nil {
		responderError(w, 400, err.Error())
		return
	}
	w.WriteHeader(http.StatusCreated)
	responder(w, a)
}

// borrarAgenda atiende DELETE /api/agenda/{id}.
// 404 en español si no existe (igual que borrarCarpeta).
func (s *Servidor) borrarAgenda(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	borrada, err := db.BorrarAgenda(s.Base, id)
	if err != nil {
		responderError(w, 500, "No se pudo borrar la nota")
		return
	}
	if !borrada {
		responderError(w, 404, "Nota no encontrada")
		return
	}
	responder(w, map[string]bool{"ok": true})
}

// actividad atiende GET /api/actividad?desde=&hasta=.
// Devuelve [{"Fecha":"YYYY-MM-DD","Total":n}] (claves Capitalizadas,
// como todo lo que sale de db: la UI las normaliza).
func (s *Servidor) actividad(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	items, err := db.ActividadPorDia(s.Base, q.Get("desde"), q.Get("hasta"))
	if err != nil {
		responderError(w, 500, "No se pudo leer la actividad")
		return
	}
	responder(w, items)
}
