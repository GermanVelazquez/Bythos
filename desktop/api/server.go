package api

// server.go — El CEREBRO de Bythos.
// La UI (ventana) y la extensión NO tocan SQLite directo.
// Le hablan a este servidor en http://localhost:8080 por HTTP + JSON.
//
// ¿Por qué net/http estándar y no Gin/Echo?
// Porque para localhost con 7 rutas, un framework es peso muerto:
// más dependencias = .exe más gordo y más cosas que aprender.
// Lo estándar te lo sabes de memoria y compila en todos lados.
//
// ¿Por qué SIN login/JWT como antes?
// Porque es tu PC, un solo usuario. El .db ya es tuyo.
// OJO: "sin login" no es "sin defensas". Cualquier pestaña que tengas
// abierta puede intentar hablarle a este puerto (ver conGuardia más abajo):
// el candado real es filtrar QUIÉN puede tocar la puerta, no ponerle
// otra puerta adentro.

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"os"
	"strconv"
	"strings"

	"bythos-desktop/db"
)

// Version es la única fuente en Go. Unifica App.jsx, package.json y manifest.
// Al cambiarla, cambia también esos 3 lugares (son 4 líneas en total).
const Version = "1.0.3"

// Servidor guarda lo único que necesita: la conexión ya abierta.
// NO abre otra (recuerda: 1 conexión para no bloquear el .db).
// UI es opcional: si main.go incrusta ui/dist, viene lleno;
// si es nil, montarUI lee del disco (dev).
type Servidor struct {
	Base *sql.DB
	UI   fs.FS
}

// Rutas arma el menú HTTP. Leerlo es leer el producto:
// - salud: ¿sigo vivo? (la UI lo usa al arrancar)
// - carpetas: crear y listar temas
// - recursos: guardar link, listar, cambiar estado, borrar
// - agenda: notas del calendario + actividad (puntos de historia)
func (s *Servidor) Rutas() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/salud", s.salud)

	mux.HandleFunc("GET /api/carpetas", s.listarCarpetas)
	mux.HandleFunc("POST /api/carpetas", s.crearCarpeta)
	mux.HandleFunc("DELETE /api/carpetas/{id}", s.borrarCarpeta)

	mux.HandleFunc("GET /api/recursos", s.listarRecursos)
	mux.HandleFunc("POST /api/recursos", s.guardarRecurso)
	mux.HandleFunc("PATCH /api/recursos/{id}", s.cambiarEstado)
	mux.HandleFunc("DELETE /api/recursos/{id}", s.borrarRecurso)

	// Agenda: calendario de la vista Todos (rango) + historia (actividad).
	mux.HandleFunc("GET /api/agenda", s.listarAgenda)
	mux.HandleFunc("POST /api/agenda", s.crearAgenda)
	mux.HandleFunc("DELETE /api/agenda/{id}", s.borrarAgenda)
	mux.HandleFunc("GET /api/actividad", s.actividad)
	mux.HandleFunc("POST /api/agenda/import", s.importarAgenda)
	// Lotes: el MD importado persiste como fuente editable/eliminable.
	mux.HandleFunc("GET /api/agenda/imports", s.listarLotes)
	mux.HandleFunc("GET /api/agenda/imports/{id}", s.obtenerLote)
	mux.HandleFunc("PUT /api/agenda/imports/{id}", s.actualizarLote)
	mux.HandleFunc("DELETE /api/agenda/imports/{id}", s.borrarLote)

	// Termómetro: % por carpeta (barra) y general (gráfico portada).
	// Van ANTES de montarUI: son /api/... (específico gana a "/" general).
	mux.HandleFunc("GET /api/carpetas/{id}/progreso", s.progresoCarpeta)
	mux.HandleFunc("GET /api/stats", s.statsGeneral)

	// Repaso: exporta tu carpeta (?format=markdown|gemini|notebooklm|drive).
	// También /api/...: antes de montarUI, misma razón.
	mux.HandleFunc("GET /api/carpetas/{id}/export", s.exportarCarpeta)
	mux.HandleFunc("POST /api/carpetas/{id}/import-avance", s.importarAvance)

	// Historial: quién tocó los datos (app, extensión o un agente de IA
	// por MCP) y qué cambió. Ver db/eventos.go y api/origen.go.
	mux.HandleFunc("GET /api/eventos", s.listarEventos)

	// Despertar la UI: "/" sirve ui/dist SIN pisar "/api/..."
	// Va AL FINAL porque "/" es el cajón de sastre. Si la pones arriba,
	// se traga la API. Orden = especifico primero, general al final.
	s.montarUI(mux)

	// Guardia de Host+Origin, no CORS abierto: cualquier web que abras
	// puede mandar fetch() a este puerto (CSRF) o resolver un dominio a
	// 127.0.0.1 (DNS rebinding) — "es localhost" no te protege de eso.
	// Por eso se rechaza Host ajeno y se responde 403 sin ACAO al Origin
	// que no está en la lista, en vez de contestar "*" a cualquiera.
	return conGuardia(mux)
}

// --- Carpetas ---

func (s *Servidor) salud(w http.ResponseWriter, r *http.Request) {
	responder(w, map[string]interface{}{"ok": true, "version": Version})
}

func (s *Servidor) listarCarpetas(w http.ResponseWriter, r *http.Request) {
	carpetas, err := db.ListarCarpetas(s.Base)
	if err != nil {
		responderError(w, 500, "No se pudieron leer las carpetas")
		return
	}
	responder(w, carpetas)
}

func (s *Servidor) crearCarpeta(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Nombre string `json:"nombre"`
	}
	if err := leerJSON(w, r, &body); err != nil {
		if esBodyDemasiadoGrande(err) {
			responderError(w, http.StatusRequestEntityTooLarge, "El body es demasiado grande (máximo 1MB)")
			return
		}
		responderError(w, 400, "JSON inválido. Manda {\"nombre\":\"React\"}")
		return
	}
	c, err := db.CrearCarpeta(s.Base, body.Nombre)
	if err != nil {
		responderError(w, 400, "Nombre inválido o carpeta duplicada")
		return
	}
	s.registrarEvento(r, db.AccionCarpetaCreada, c.ID, c.Nombre)
	w.WriteHeader(http.StatusCreated)
	responder(w, c)
}

func (s *Servidor) borrarCarpeta(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	nombre := nombreCarpetaPorID(s, id) // antes de borrar: después ya no existe
	borrada, err := db.BorrarCarpeta(s.Base, id)
	if err != nil {
		responderError(w, 500, "No se pudo borrar la carpeta")
		return
	}
	if !borrada {
		responderError(w, 404, "Carpeta no encontrada")
		return
	}
	s.registrarEvento(r, db.AccionCarpetaBorrada, id, nombre)
	responder(w, map[string]bool{"ok": true})
}

// --- Recursos ---

func (s *Servidor) listarRecursos(w http.ResponseWriter, r *http.Request) {
	// ?carpeta_id=3 filtra, sin param trae todos (igual que db.ListarRecursos)
	var carpetaID int64
	if q := r.URL.Query().Get("carpeta_id"); q != "" {
		carpetaID, _ = strconv.ParseInt(q, 10, 64)
	}
	recursos, err := db.ListarRecursos(s.Base, carpetaID)
	if err != nil {
		responderError(w, 500, "No se pudieron leer los recursos")
		return
	}
	responder(w, recursos)
}

func (s *Servidor) guardarRecurso(w http.ResponseWriter, r *http.Request) {
	// Este es EL botón 💾 Guardar: la UI y la extensión mandan esto.
	// Fíjate que NO pide estado: nace pendiente por regla en db.Guardar.
	var body struct {
		CarpetaID   int64  `json:"carpeta_id"`
		URL         string `json:"url"`
		Titulo      string `json:"titulo"`
		Imagen      string `json:"imagen"`
		Descripcion string `json:"descripcion"`
		Tipo        string `json:"tipo"`
	}
	if err := leerJSON(w, r, &body); err != nil {
		if esBodyDemasiadoGrande(err) {
			responderError(w, http.StatusRequestEntityTooLarge, "El body es demasiado grande (máximo 1MB)")
			return
		}
		responderError(w, 400, "JSON inválido")
		return
	}
	// OLFATO: si la UI/extensión mandó SOLO {url} (caso normal),
	// rellenamos título/imagen/descripción/tipo sin pedir nada más.
	// Si ya mandó título manual, se respeta (no se pisa).
	if body.Titulo == "" || body.Tipo == "" {
		m := obtenerMetadata(body.URL)
		if body.Titulo == "" {
			body.Titulo = m.Titulo
		}
		if body.Imagen == "" {
			body.Imagen = m.Imagen
		}
		if body.Descripcion == "" {
			body.Descripcion = m.Descripcion
		}
		if body.Tipo == "" {
			body.Tipo = m.Tipo
		}
	}
	rec, err := db.Guardar(s.Base, body.CarpetaID, body.URL, body.Titulo, body.Imagen, body.Descripcion, body.Tipo)
	if err != nil {
		responderError(w, 400, "Falta url o carpeta_id, o la carpeta no existe")
		return
	}
	s.registrarEvento(r, db.AccionRecursoGuardado, rec.ID, tituloOUrl(rec))
	w.WriteHeader(http.StatusCreated)
	responder(w, rec)
}

func (s *Servidor) cambiarEstado(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	var body struct {
		Estado   string `json:"estado"`
		Progreso *int   `json:"progreso"`
	}
	if err := leerJSON(w, r, &body); err != nil && esBodyDemasiadoGrande(err) {
		responderError(w, http.StatusRequestEntityTooLarge, "El body es demasiado grande (máximo 1MB)")
		return
	}
	// Leído ANTES de tocar nada: es el "antes" que el historial necesita
	// para mostrar "Título": 40% → 80% (ver db.ObtenerRecurso). previo.ok
	// en false (recurso inexistente) no bloquea nada: los Actualizar*/
	// CambiarEstado de abajo ya fallan solos, este read es solo para el detalle.
	previo, previoOk, _ := db.ObtenerRecurso(s.Base, id)
	// Progress-first: {"progreso":N} syncs estado for compatibility.
	// {"estado":...} still works alone and wins when both are sent.
	if body.Progreso != nil {
		if err := db.ActualizarProgreso(s.Base, id, *body.Progreso); err != nil {
			responderError(w, 500, "No se pudo actualizar el progreso")
			return
		}
		if previoOk {
			detalle := tituloOUrl(previo) + ": " + strconv.Itoa(previo.Progreso) + "% → " + strconv.Itoa(*body.Progreso) + "%"
			s.registrarEvento(r, db.AccionProgresoCambiado, id, detalle)
		}
	}
	if body.Estado != "" {
		if err := db.CambiarEstado(s.Base, id, body.Estado); err != nil {
			responderError(w, 400, "Estado inválido. Usa pendiente, en_curso o completado")
			return
		}
		if previoOk {
			detalle := tituloOUrl(previo) + ": " + previo.Estado + " → " + body.Estado
			s.registrarEvento(r, db.AccionEstadoCambiado, id, detalle)
		}
	}
	if body.Progreso == nil && body.Estado == "" {
		responderError(w, 400, "Manda {\"estado\":\"...\"} o {\"progreso\":N}")
		return
	}
	responder(w, map[string]bool{"ok": true})
}

func (s *Servidor) borrarRecurso(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	previo, previoOk, _ := db.ObtenerRecurso(s.Base, id) // antes de borrar: después ya no existe
	if err := db.Borrar(s.Base, id); err != nil {
		responderError(w, 500, "No se pudo borrar")
		return
	}
	if previoOk {
		s.registrarEvento(r, db.AccionRecursoBorrado, id, tituloOUrl(previo))
	}
	responder(w, map[string]bool{"ok": true})
}

// --- Progreso (termómetro) ---

// GET /api/carpetas/{id}/progreso → {total, completados, pendientes, en_curso, porcentaje}
// La barra de cada carpeta en la UI bebe de aquí.
func (s *Servidor) progresoCarpeta(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	p, err := db.ProgresoCarpeta(s.Base, id)
	if err != nil {
		responderError(w, 500, "No se pudo calcular el progreso")
		return
	}
	responder(w, p)
}

// GET /api/stats → lo mismo pero de TODO (gráfico de portada).
func (s *Servidor) statsGeneral(w http.ResponseWriter, r *http.Request) {
	p, err := db.ProgresoGeneral(s.Base)
	if err != nil {
		responderError(w, 500, "No se pudo calcular el progreso")
		return
	}
	responder(w, p)
}

// --- Historial ---

// GET /api/eventos?limite=&origen= → el historial, más nuevo primero.
// Claves en minúscula (a diferencia de carpetas/recursos/agenda): esta
// ruta nace después de ellas y no tiene UI vieja que espere Capitalizado.
func (s *Servidor) listarEventos(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	filtro := db.FiltroEventos{Origen: q.Get("origen")}
	if v := q.Get("limite"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			filtro.Limite = n
		}
	}
	eventos, err := db.ListarEventos(s.Base, filtro)
	if err != nil {
		responderError(w, 500, "No se pudo leer el historial")
		return
	}
	out := make([]map[string]interface{}, 0, len(eventos))
	for _, e := range eventos {
		out = append(out, map[string]interface{}{
			"id": e.ID, "creado": e.Creado, "origen": e.Origen, "actor": e.Actor,
			"accion": e.Accion, "entidad_id": e.EntidadID, "detalle": e.Detalle,
		})
	}
	responder(w, out)
}

// --- Helpers (siempre igual, cópialos de memoria) ---

// nombreCarpetaPorID busca el nombre de una carpeta para el detalle del
// historial. "" si no existe o si falla la lectura: el borrado real ya
// tiene su propio manejo de error, este helper es solo cosmético.
func nombreCarpetaPorID(s *Servidor, id int64) string {
	carpetas, err := db.ListarCarpetas(s.Base)
	if err != nil {
		return ""
	}
	for _, c := range carpetas {
		if c.ID == id {
			return c.Nombre
		}
	}
	return ""
}

// tituloOUrl es el nombre legible de un recurso para el historial:
// título si lo tiene, la URL si no (mismo criterio que tituloDe en export.go).
func tituloOUrl(r db.Recurso) string {
	if r.Titulo != "" {
		return r.Titulo
	}
	return r.URL
}

// limiteBodyJSON es el techo de tamaño para cualquier body JSON que la API
// decodifica. 1MB sobra para nombres, urls o notas cortas; sin techo, un
// cliente hostil (o buggy) puede tirar el proceso mandando gigabytes al
// Decode antes de que el JSON siquiera se valide.
const limiteBodyJSON = 1 << 20

// leerJSON decodifica el body de la request con el techo de arriba
// (http.MaxBytesReader). Los handlers la usan en vez de
// json.NewDecoder(r.Body).Decode directo, así el límite es uno solo y no
// hay que repetirlo en cada ruta.
// JSON inválido sigue devolviendo el mismo error de Decode que antes (cada
// handler lo traduce a su 400 de siempre); solo cuando el body excede el
// techo, err es un *http.MaxBytesError que esBodyDemasiadoGrande reconoce.
func leerJSON(w http.ResponseWriter, r *http.Request, destino interface{}) error {
	r.Body = http.MaxBytesReader(w, r.Body, limiteBodyJSON)
	return json.NewDecoder(r.Body).Decode(destino)
}

// esBodyDemasiadoGrande distingue el 413 (body por encima del techo) de un
// 400 común de JSON mal formado.
func esBodyDemasiadoGrande(err error) bool {
	var demasiadoGrande *http.MaxBytesError
	return errors.As(err, &demasiadoGrande)
}

func responder(w http.ResponseWriter, dato interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(dato)
}

func responderError(w http.ResponseWriter, codigo int, mensaje string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(codigo)
	json.NewEncoder(w).Encode(map[string]string{"error": mensaje})
}

// hostsPermitidos son los ÚNICOS Host: que se aceptan. Sin esto, un dominio
// atacante que resuelva a 127.0.0.1 (DNS rebinding) sería "same-origin" para
// el navegador aunque el Host: que llega diga otra cosa (ej. evil.com:8080).
var hostsPermitidos = map[string]bool{
	"localhost:8080": true, // ventana (ventana/) y curl/esperarSalud directo
	"127.0.0.1:8080": true, // mismo server, por IP en vez de nombre
}

// origenesPermitidos son los orígenes exactos que pueden LEER la respuesta.
// chrome-extension:// se compara por prefijo porque el id de una extensión
// "unpacked" (dev, sin firmar) cambia en cada instalación.
var origenesPermitidos = []string{
	"http://localhost:8080", // UI embebida, misma ventana que sirve Go
}

// Vite dev (pnpm dev) sirve la UI en :5173 y proxea a :8080 sin
// changeOrigin: llegan Host y Origin de :5173. Solo se aceptan con
// BYTHOS_DEV=1. En un release, :5173 es el puerto por defecto de
// CUALQUIER proyecto Vite: aceptarlo dejaría que una página ajena en ese
// puerto lea tu biblioteca o abra la terminal del agente.
const (
	hostVite   = "localhost:5173"
	origenVite = "http://localhost:5173"
)

func modoDev() bool { return os.Getenv("BYTHOS_DEV") == "1" }

func hostPermitido(host string) bool {
	return hostsPermitidos[host] || (modoDev() && host == hostVite)
}

func origenPermitido(origin string) bool {
	if origin == "" {
		return false
	}
	if strings.HasPrefix(origin, "chrome-extension://") {
		return true
	}
	if modoDev() && origin == origenVite {
		return true
	}
	for _, o := range origenesPermitidos {
		if origin == o {
			return true
		}
	}
	return false
}

// conGuardia reemplaza el CORS abierto de antes. Deja pasar a la ventana,
// a Vite dev y a la extensión, y RECHAZA todo lo demás en vez de confiar
// en el navegador: un POST con Content-Type: text/plain (que salta el
// preflight) igual llega aquí, así que la defensa tiene que estar en el
// servidor, no solo en los headers de CORS.
func conGuardia(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !hostPermitido(r.Host) {
			responderError(w, http.StatusForbidden, "Host no permitido")
			return
		}

		// Sin Origin (curl, esperarSalud, navegación same-origin normal):
		// no es una lectura cross-site, se deja pasar sin cabeceras CORS.
		if origin := r.Header.Get("Origin"); origin != "" {
			if !origenPermitido(origin) {
				responderError(w, http.StatusForbidden, "Origen no permitido")
				return
			}
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Bythos-Origen, X-Bythos-Actor")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		}

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
