package api

// origen.go — quién tocó los datos. Cada cliente (UI, extensión, agente
// MCP) manda X-Bythos-Origen (+ X-Bythos-Actor opcional) y este archivo
// las traduce a lo que pide db.RegistrarEvento (ver db/eventos.go).
//
// ¿Por qué cabeceras y no un token de sesión? Porque Bythos no tiene
// login: conGuardia (Host+Origin) ya es el candado real. Estas cabeceras
// NO autorizan nada, solo etiquetan la fila del historial para que
// puedas auditar qué hizo un agente de IA sobre tus datos.
//
// ¿Por qué nunca rechazar la petición por un origen raro? Porque el
// historial es auditoría, no seguridad: perder un guardado real por una
// cabecera mal formada sería peor que anotarlo como "desconocido".

import (
	"log"
	"net/http"
	"strings"

	"bythos-desktop/db"
)

// actorLargoMaximo evita que un actor gigantesco (a propósito o no)
// infle una fila del historial; 80 caracteres alcanza de sobra para el
// nombre de un cliente MCP o de un agente de IA.
const actorLargoMaximo = 80

// origenDeCabeceras lee X-Bythos-Origen. Cualquier valor que no sea
// exactamente app/extension/agente (sin importar mayúsculas) cae en
// db.OrigenDesconocido, incluida la ausencia de la cabecera.
func origenDeCabeceras(r *http.Request) string {
	crudo := strings.ToLower(strings.TrimSpace(r.Header.Get("X-Bythos-Origen")))
	switch crudo {
	case db.OrigenApp, db.OrigenExtension, db.OrigenAgente:
		return crudo
	default:
		return db.OrigenDesconocido
	}
}

// actorDeCabeceras lee X-Bythos-Actor, recortado a un largo sano.
// Vacío es válido: no todo origen manda un actor (la UI y la extensión,
// por ejemplo, no tienen "nombre de cliente" que mandar).
func actorDeCabeceras(r *http.Request) string {
	actor := strings.TrimSpace(r.Header.Get("X-Bythos-Actor"))
	if len(actor) > actorLargoMaximo {
		actor = actor[:actorLargoMaximo]
	}
	return actor
}

// registrarEvento deja rastro de una mutación ya aplicada con éxito.
// Un fallo al escribir el historial NUNCA debe romper la acción del
// usuario: solo queda en stderr (log.Println), la respuesta HTTP ya
// salió (o está por salir) con su 200/201 normal.
func (s *Servidor) registrarEvento(r *http.Request, accion string, entidadID int64, detalle string) {
	origen := origenDeCabeceras(r)
	actor := actorDeCabeceras(r)
	if err := db.RegistrarEvento(s.Base, origen, actor, accion, entidadID, detalle); err != nil {
		log.Println("No se pudo registrar evento de historial:", err)
	}
}
