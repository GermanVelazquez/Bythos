package api

// sesiones.go — /api/terminal: sesiones de Terminal, su pizarra y el entorno.
// Cada ruta pasa primero por conGuardia (Host) y luego por UNA de las dos
// guardas de abajo (design D8). PR3 y PR6a las reusan para el resto de rutas.

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"os/exec"
	"runtime"
	"strconv"
	"strings"

	"bythos-desktop/db"
	"bythos-desktop/terminal"
)

// limiteEscena: techo propio del PUT de la pizarra (design D10).
const limiteEscena = 20 << 20

// gestorTerminal: lo que esta API necesita del gestor de PTYs (*terminal.Gestor
// lo cumple). Servidor.Terminal en nil = sin PTYs; nunca le asignes un
// *terminal.Gestor nil (sería un interface no-nil con puntero nil).
type gestorTerminal interface {
	Adjuntar(id int64, cfg func() (terminal.Config, error), cols, filas uint16) (*terminal.Adjunto, error)
	Cerrar(id int64) error
	Vivas() []int64
}

// origenVentanaOK: el Origin EXACTO de la ventana (o Vite en modo dev).
func origenVentanaOK(origin string) bool {
	return origenesAgentePermitidos[origin] || (modoDev() && origin == origenVite)
}

// conVentana (guarda "Ventana", para POST/PUT/PATCH/DELETE y WS): exige el
// Origin exacto de la ventana. Rechaza Origin vacío y chrome-extension://,
// que conGuardia sí admite.
func conVentana(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !origenVentanaOK(r.Header.Get("Origin")) {
			prohibido(w)
			return
		}
		next(w, r)
	}
}

func prohibido(w http.ResponseWriter) { responderError(w, http.StatusForbidden, "Origen no permitido") }

// conVentanaLectura (guarda "Ventana-lectura", para GETs de la UI): el
// navegador no manda Origin en un GET same-origin, así que vacío se admite;
// pero cualquier Origin presente debe ser el exacto (esto deja fuera la
// extensión) y Sec-Fetch-Site, si viene, debe ser same-origin (corta lecturas
// cross-site como esOrigenCruzado en archivos.go, pero sin admitir "none").
func conVentanaLectura(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		o, f := r.Header.Get("Origin"), r.Header.Get("Sec-Fetch-Site")
		if (o != "" && !origenVentanaOK(o)) || (f != "" && f != "same-origin") {
			prohibido(w)
			return
		}
		next(w, r)
	}
}

func (s *Servidor) rutasSesiones(mux *http.ServeMux) {
	const p = "/api/terminal/"
	mux.HandleFunc("GET "+p+"sesiones", conVentanaLectura(s.listarSesiones))
	mux.HandleFunc("GET "+p+"sesiones/{id}/pizarra", conVentanaLectura(s.leerPizarra))
	mux.HandleFunc("GET "+p+"entorno", conVentanaLectura(s.entornoTerminal))
	mux.HandleFunc("POST "+p+"sesiones", conVentana(s.crearSesion))
	mux.HandleFunc("PATCH "+p+"sesiones/{id}", conVentana(s.cambiarSesion))
	mux.HandleFunc("DELETE "+p+"sesiones/{id}", conVentana(s.borrarSesion))
	mux.HandleFunc("PUT "+p+"sesiones/{id}/pizarra", conVentana(s.guardarPizarra))
	s.rutasTerminal(mux)
}

var modosSesion = map[string]bool{"practica": true, "revision": true, "prototipo": true, "dibujo": true, "paginas": true}

// validarSesion devuelve el mensaje del 400, o "" si todo está bien. Los
// punteros nil (PATCH sin ese campo) no se validan.
func validarSesion(nombre, modo *string) string {
	if nombre != nil && strings.TrimSpace(*nombre) == "" {
		return "Escribe un nombre para la sesión."
	}
	if modo != nil && !modosSesion[*modo] {
		return "Modo inválido"
	}
	return ""
}

// errorSesion: una carpeta inexistente rompe la llave foránea (400); lo demás es 500.
func errorSesion(w http.ResponseWriter, err error, msg500 string) {
	if strings.Contains(err.Error(), "FOREIGN KEY") {
		responderError(w, 400, "La carpeta no existe")
		return
	}
	responderError(w, 500, msg500)
}

// idSesion lee {id}; ok false ya respondió el 404 ("La sesión N no existe").
func idSesion(w http.ResponseWriter, r *http.Request) (int64, bool) {
	raw := r.PathValue("id")
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		responderError(w, http.StatusNotFound, "La sesión "+raw+" no existe")
		return 0, false
	}
	return id, true
}

func noExiste(w http.ResponseWriter, id int64) {
	responderError(w, http.StatusNotFound, "La sesión "+strconv.FormatInt(id, 10)+" no existe")
}

func (s *Servidor) listarSesiones(w http.ResponseWriter, r *http.Request) {
	lista, err := db.ListarSesiones(s.Base)
	if err != nil {
		responderError(w, 500, "No se pudieron leer las sesiones")
		return
	}
	responder(w, lista)
}

func (s *Servidor) crearSesion(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Nombre    string `json:"nombre"`
		Modo      string `json:"modo"`
		CarpetaID *int64 `json:"carpeta_id"`
	}
	if err := leerJSON(w, r, &body); err != nil {
		if esBodyDemasiadoGrande(err) {
			responderError(w, http.StatusRequestEntityTooLarge, "El body es demasiado grande (máximo 1MB)")
			return
		}
		responderError(w, 400, "JSON inválido")
		return
	}
	if msg := validarSesion(&body.Nombre, &body.Modo); msg != "" {
		responderError(w, 400, msg)
		return
	}
	se, err := db.CrearSesion(s.Base, body.Nombre, body.Modo, body.CarpetaID)
	if err != nil {
		errorSesion(w, err, "No se pudo crear la sesión")
		return
	}
	s.registrarEvento(r, db.AccionSesionTerminalCreada, se.ID, se.Nombre)
	responderCreado(w, se)
}

func (s *Servidor) cambiarSesion(w http.ResponseWriter, r *http.Request) {
	id, ok := idSesion(w, r)
	if !ok {
		return
	}
	var body struct {
		Nombre    *string         `json:"nombre"`
		Modo      *string         `json:"modo"`
		CarpetaID json.RawMessage `json:"carpeta_id"` // ausente = no tocar; null = soltar
	}
	if err := leerJSON(w, r, &body); err != nil {
		responderError(w, 400, "JSON inválido")
		return
	}
	c := db.CambiosSesion{Nombre: body.Nombre, Modo: body.Modo}
	if msg := validarSesion(body.Nombre, body.Modo); msg != "" {
		responderError(w, 400, msg)
		return
	}
	if body.CarpetaID != nil {
		c.CarpetaSet = true
		if string(body.CarpetaID) != "null" {
			var cid int64
			if json.Unmarshal(body.CarpetaID, &cid) != nil {
				responderError(w, 400, "carpeta_id inválido")
				return
			}
			c.CarpetaID = &cid
		}
	}
	hecho, err := db.ActualizarSesion(s.Base, id, c)
	if err != nil {
		errorSesion(w, err, "No se pudo actualizar la sesión")
		return
	}
	if !hecho {
		noExiste(w, id)
		return
	}
	responder(w, map[string]bool{"ok": true})
}

// borrarSesion cierra el PTY ANTES de borrar la fila: si el borrado fallara,
// no queda un proceso vivo sin sesión que lo reclame.
func (s *Servidor) borrarSesion(w http.ResponseWriter, r *http.Request) {
	id, ok := idSesion(w, r)
	if !ok {
		return
	}
	se, existe, err := db.ObtenerSesion(s.Base, id)
	if err != nil {
		responderError(w, 500, "No se pudo leer la sesión")
		return
	}
	if !existe {
		noExiste(w, id)
		return
	}
	if s.Terminal != nil {
		if err := s.Terminal.Cerrar(id); err != nil {
			responderError(w, 500, "No se pudo cerrar la terminal de la sesión")
			return
		}
	}
	if _, err := db.BorrarSesion(s.Base, id); err != nil {
		responderError(w, 500, "No se pudo borrar la sesión")
		return
	}
	s.registrarEvento(r, db.AccionSesionTerminalBorrada, id, se.Nombre)
	responder(w, map[string]bool{"ok": true})
}

func (s *Servidor) leerPizarra(w http.ResponseWriter, r *http.Request) {
	id, ok := idSesion(w, r)
	if !ok {
		return
	}
	escena, version, existe, err := db.ObtenerPizarra(s.Base, id)
	if err != nil {
		responderError(w, 500, "No se pudo leer la pizarra")
		return
	}
	if !existe {
		noExiste(w, id)
		return
	}
	if escena == "" {
		escena = db.EscenaVacia
	}
	responder(w, map[string]interface{}{"escena": json.RawMessage(escena), "version": version})
}

// reducirEscena deja solo {elements (sin isDeleted), appState:{viewBackgroundColor},
// files:{}} (design D10). permitidos = sha256 de los adjuntos de la sesión (D9):
// ok false = JSON inválido, imagen o tarjeta que no apunta a un adjunto propio,
// o embeddable/iframe (400, falla cerrado).
func reducirEscena(raw json.RawMessage, permitidos map[string]bool) (string, bool) {
	var e struct {
		Elements []json.RawMessage `json:"elements"`
		AppState struct {
			Fondo string `json:"viewBackgroundColor"`
		} `json:"appState"`
	}
	if json.Unmarshal(raw, &e) != nil {
		return "", false
	}
	elementos := make([]json.RawMessage, 0, len(e.Elements))
	for _, el := range e.Elements {
		borrado, ok := validarElemento(el, permitidos)
		if !ok {
			return "", false
		}
		if !borrado {
			elementos = append(elementos, el)
		}
	}
	if e.AppState.Fondo == "" {
		e.AppState.Fondo = "#ffffff"
	}
	salida, err := json.Marshal(map[string]interface{}{
		"elements": elementos,
		"appState": map[string]string{"viewBackgroundColor": e.AppState.Fondo},
		"files":    map[string]string{},
	})
	return string(salida), err == nil
}

func (s *Servidor) guardarPizarra(w http.ResponseWriter, r *http.Request) {
	id, ok := idSesion(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, limiteEscena)
	var body struct {
		Escena  json.RawMessage `json:"escena"`
		Version int64           `json:"version"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		if esBodyDemasiadoGrande(err) {
			responderError(w, http.StatusRequestEntityTooLarge, "La pizarra es demasiado grande (máximo 20MB)")
			return
		}
		responderError(w, 400, "JSON inválido. Manda {\"escena\":{...},\"version\":N}")
		return
	}
	adjuntos, err := db.ListarAdjuntos(s.Base, id)
	if err != nil {
		responderError(w, 500, "No se pudo leer la pizarra")
		return
	}
	permitidos := make(map[string]bool, len(adjuntos))
	for _, a := range adjuntos {
		permitidos[a.SHA256] = true
	}
	escena, ok := reducirEscena(body.Escena, permitidos)
	if !ok {
		responderError(w, 400, "Escena inválida: referencia imágenes o tarjetas que no son adjuntos de esta sesión")
		return
	}
	nueva, err := db.GuardarPizarra(s.Base, id, escena, body.Version)
	var conflicto *db.ConflictoVersion
	switch {
	case errors.As(err, &conflicto):
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(map[string]interface{}{"error": "La pizarra cambió en otra ventana", "version": conflicto.Actual})
	case errors.Is(err, sql.ErrNoRows):
		noExiste(w, id)
	case err != nil:
		responderError(w, 500, "No se pudo guardar la pizarra")
	default:
		responder(w, map[string]interface{}{"ok": true, "version": nueva})
	}
}

// entornoTerminal: qué hay disponible. conpty = Windows (la versión exacta la
// decide el gestor al abrir el PTY, que responde 501 si no alcanza).
func (s *Servidor) entornoTerminal(w http.ResponseWriter, r *http.Request) {
	_, errEngram := exec.LookPath("engram")
	vivas := 0
	if s.Terminal != nil {
		vivas = len(s.Terminal.Vivas())
	}
	responder(w, map[string]interface{}{
		"conpty":   runtime.GOOS == "windows",
		"engram":   errEngram == nil,
		"sesiones": vivas,
	})
}

// clavesExactas: encoding/json compara claves sin distinguir mayúsculas y JS
// no; una clave que solo difiere en mayúsculas de las leídas es ambigua (400).
func clavesExactas(m map[string]json.RawMessage, leidas ...string) bool {
	for k := range m {
		for _, l := range leidas {
			if k != l && strings.EqualFold(k, l) {
				return false
			}
		}
	}
	return true
}

// referenciaPermitida: el valor debe ser un string presente en permitidos.
func referenciaPermitida(v json.RawMessage, permitidos map[string]bool) bool {
	var s string
	return json.Unmarshal(v, &s) == nil && permitidos[s]
}

// validarElemento lee solo las claves exactas type, fileId, isDeleted y
// customData.sha256 (D9). Falla cerrado: fileId o sha256 presentes en cualquier
// tipo deben ser adjuntos propios. Lista de denegados, no de permitidos, para
// no romper escenas heredadas; `link` y `frame` se admiten (el enlace lo abre
// el anfitrión con onLinkOpen y frame solo agrupa).
func validarElemento(el json.RawMessage, permitidos map[string]bool) (borrado, ok bool) {
	var m map[string]json.RawMessage
	if json.Unmarshal(el, &m) != nil || !clavesExactas(m, "type", "fileId", "isDeleted", "customData") {
		return false, false
	}
	var tipo string
	json.Unmarshal(m["type"], &tipo)
	if tipo == "embeddable" || tipo == "iframe" || tipo == "magicframe" {
		return false, false
	}
	if v, hay := m["isDeleted"]; hay && json.Unmarshal(v, &borrado) != nil {
		return false, false
	}
	if borrado {
		return true, true
	}
	if v, hay := m["fileId"]; hay && !referenciaPermitida(v, permitidos) {
		return false, false
	}
	if tipo == "image" && m["fileId"] == nil {
		return false, false
	}
	if v, hay := m["customData"]; hay && string(v) != "null" {
		var datos map[string]json.RawMessage
		if json.Unmarshal(v, &datos) != nil || !clavesExactas(datos, "sha256") {
			return false, false
		}
		if sha, hay := datos["sha256"]; hay && !referenciaPermitida(sha, permitidos) {
			return false, false
		}
	}
	return false, true
}
