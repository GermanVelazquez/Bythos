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
// Poner login a localhost es como ponerle candado a tu cajón desde adentro.

import (
	"database/sql"
	"encoding/json"
	"io/fs"
	"net/http"
	"strconv"

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

	// Despertar la UI: "/" sirve ui/dist SIN pisar "/api/..."
	// Va AL FINAL porque "/" es el cajón de sastre. Si la pones arriba,
	// se traga la API. Orden = especifico primero, general al final.
	s.montarUI(mux)

	// CORS abierto SOLO porque UI y extensión corren en otro origen
	// (ventana file://, extensión chrome-extension://).
	// En localhost es seguro; en internet sería otra historia.
	return conCORS(mux)
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
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		responderError(w, 400, "JSON inválido. Manda {\"nombre\":\"React\"}")
		return
	}
	c, err := db.CrearCarpeta(s.Base, body.Nombre)
	if err != nil {
		responderError(w, 400, "Nombre inválido o carpeta duplicada")
		return
	}
	w.WriteHeader(http.StatusCreated)
	responder(w, c)
}

func (s *Servidor) borrarCarpeta(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	borrada, err := db.BorrarCarpeta(s.Base, id)
	if err != nil {
		responderError(w, 500, "No se pudo borrar la carpeta")
		return
	}
	if !borrada {
		responderError(w, 404, "Carpeta no encontrada")
		return
	}
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
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
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
	w.WriteHeader(http.StatusCreated)
	responder(w, rec)
}

func (s *Servidor) cambiarEstado(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	var body struct {
		Estado   string `json:"estado"`
		Progreso *int   `json:"progreso"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	// Progress-first: {"progreso":N} syncs estado for compatibility.
	// {"estado":...} still works alone and wins when both are sent.
	if body.Progreso != nil {
		if err := db.ActualizarProgreso(s.Base, id, *body.Progreso); err != nil {
			responderError(w, 500, "No se pudo actualizar el progreso")
			return
		}
	}
	if body.Estado != "" {
		if err := db.CambiarEstado(s.Base, id, body.Estado); err != nil {
			responderError(w, 400, "Estado inválido. Usa pendiente, en_curso o completado")
			return
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
	if err := db.Borrar(s.Base, id); err != nil {
		responderError(w, 500, "No se pudo borrar")
		return
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

// --- Helpers (siempre igual, cópialos de memoria) ---

func responder(w http.ResponseWriter, dato interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(dato)
}

func responderError(w http.ResponseWriter, codigo int, mensaje string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(codigo)
	json.NewEncoder(w).Encode(map[string]string{"error": mensaje})
}

// conCORS deja pasar a la ventana y a la extensión.
// Sin esto el navegador bloquea todo con "CORS policy".
func conCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
