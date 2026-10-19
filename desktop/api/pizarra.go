package api

// pizarra.go — latido, ops y capturas de la pizarra (design D8 Agente, D12, D13).
// La escena la posee la UI: el agente (MCP) encola ops o pide capturas y la UI
// las recoge por latido (1 s) mientras la pantalla Terminal está montada.
// Los handlers que ESPERAN (ack 8 s, captura 15 s) no sostienen ninguna
// conexión de BD mientras esperan (SetMaxOpenConns(1)): cada llamada a db
// termina antes del select.

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"sort"
	"strconv"
	"sync"
	"time"

	"bythos-desktop/archivos"
	"bythos-desktop/db"
	"bythos-desktop/pizarra"
)

const (
	limiteAck            = 2 << 20   // el ack con inversa se lee con su propio techo (D13)
	topeInversa          = 512 << 10 // una inversa mayor se descarta: el lote queda sin deshacer
	esperaOpsDefecto     = 8 * time.Second
	esperaCapturaDefecto = 15 * time.Second
	vigenciaLatido       = 3 * time.Second // sin latido en este tiempo, la pizarra no está montada
	limitePNG            = 4 << 20
	limiteOps            = 8 << 20 // 200 ops con texto de 10,000 no caben en limiteBodyJSON
	mensajePendiente     = "se aplicará en segundos si la pizarra sigue abierta"
	msgPizarraCerrada    = "La pizarra de esta sesión no está abierta: abre la pizarra en la pantalla Terminal de Bythos y vuelve a intentarlo."
	msgTerminalCerrada   = "La pantalla Terminal no está abierta: ábrela en Bythos para capturar la pizarra."
)

var firmaPNG = []byte("\x89PNG\r\n\x1a\n")

type resultadoOp struct {
	Aplicada bool
	IDs      []string
	Error    string
}

type capturaPendiente struct {
	sesion    int64
	png       chan []byte
	entregada bool // ya salió por un latido (se entrega una sola vez)
}

// estadoPizarra vive solo en memoria (se pierde al reiniciar, a propósito).
// Valor cero listo para usar; ahora/espera* nil/0 = reloj y tiempos reales
// (los tests los inyectan).
type estadoPizarra struct {
	mu                       sync.Mutex
	latidos                  map[int64]time.Time
	ultima                   int64 // sesión del último latido de este proceso
	ops                      map[int64]chan resultadoOp
	capturas                 map[int64]*capturaPendiente
	sigCaptura               int64
	ahora                    func() time.Time
	esperaOps, esperaCaptura time.Duration
}

func (p *estadoPizarra) hora() time.Time {
	if p.ahora != nil {
		return p.ahora()
	}
	return time.Now()
}

func espera(valor, defecto time.Duration) time.Duration {
	if valor > 0 {
		return valor
	}
	return defecto
}

func (p *estadoPizarra) latir(sesion int64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.latidos == nil {
		p.latidos = map[int64]time.Time{}
	}
	p.latidos[sesion], p.ultima = p.hora(), sesion
}

func (p *estadoPizarra) montada(sesion int64) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	t, ok := p.latidos[sesion]
	return ok && p.hora().Sub(t) <= vigenciaLatido
}

func (p *estadoPizarra) ultimaSesion() int64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.ultima
}

// esperarOp registra quién espera el ack de la op (buffer 1: avisar nunca bloquea).
func (p *estadoPizarra) esperarOp(id int64) chan resultadoOp {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.ops == nil {
		p.ops = map[int64]chan resultadoOp{}
	}
	p.ops[id] = make(chan resultadoOp, 1)
	return p.ops[id]
}

func (p *estadoPizarra) soltarOp(id int64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.ops, id)
}

func (p *estadoPizarra) avisarOp(id int64, r resultadoOp) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if c, ok := p.ops[id]; ok {
		select {
		case c <- r:
		default:
		}
	}
}

func (p *estadoPizarra) nuevaCaptura(sesion int64) (int64, chan []byte) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.capturas == nil {
		p.capturas = map[int64]*capturaPendiente{}
	}
	p.sigCaptura++
	c := &capturaPendiente{sesion: sesion, png: make(chan []byte, 1)}
	p.capturas[p.sigCaptura] = c
	return p.sigCaptura, c.png
}

// capturasPara devuelve (una sola vez) las capturas que la UI de la sesión debe hacer.
func (p *estadoPizarra) capturasPara(sesion int64) []int64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	ids := []int64{}
	for id, c := range p.capturas {
		if c.sesion == sesion && !c.entregada {
			c.entregada = true
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

func (p *estadoPizarra) hayCaptura(id int64) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.capturas[id] != nil
}

// entregarCaptura reclama la captura (la primera subida gana) y le pasa el PNG a quien espera.
func (p *estadoPizarra) entregarCaptura(id int64, png []byte) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	c := p.capturas[id]
	if c == nil {
		return false
	}
	delete(p.capturas, id)
	c.png <- png
	return true
}

func (p *estadoPizarra) soltarCaptura(id int64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.capturas, id)
}

// conAgente (guarda "Agente", design D8): el proceso MCP no manda Origin, así
// que vacío se admite; cualquier Origin presente debe ser el exacto de la
// ventana. Esto deja fuera chrome-extension:// y cualquier página ajena.
func conAgente(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if o := r.Header.Get("Origin"); o != "" && !origenVentanaOK(o) {
			prohibido(w)
			return
		}
		next(w, r)
	}
}

func (s *Servidor) rutasPizarra(mux *http.ServeMux) {
	const p = "/api/terminal/"
	mux.HandleFunc("GET "+p+"pizarra", conAgente(s.leerPizarraAgente))
	mux.HandleFunc("POST "+p+"pizarra/ops", conAgente(s.encolarOps))
	mux.HandleFunc("POST "+p+"pizarra/marcas", conAgente(s.marcarPizarra))
	mux.HandleFunc("POST "+p+"pizarra/captura", conAgente(s.pedirCaptura))
	mux.HandleFunc("POST "+p+"latido", conVentana(s.latido))
	mux.HandleFunc("POST "+p+"pizarra/ops/{id}/ack", conVentana(s.ackOp))
	mux.HandleFunc("GET "+p+"sesiones/{id}/cambios", conVentanaLectura(s.listarCambios))
	mux.HandleFunc("POST "+p+"pizarra/capturas/{id}", conVentana(s.subirCaptura))
}

// sesionPorTexto busca la sesión por un id escrito; no numérico = no existe.
func (s *Servidor) sesionPorTexto(texto string) (db.Sesion, bool, error) {
	id, err := strconv.ParseInt(texto, 10, 64)
	if err != nil {
		return db.Sesion{}, false, nil
	}
	return db.ObtenerSesion(s.Base, id)
}

// resolverSesion: argumento (autoridad) > pista (BYTHOS_SESION) > último
// latido > la más reciente. ok false ya respondió el 404/500.
func (s *Servidor) resolverSesion(w http.ResponseWriter, arg, pista string) (se db.Sesion, resolucion string, ok bool) {
	fallo := func() (db.Sesion, string, bool) {
		responderError(w, 500, "No se pudo leer la sesión")
		return db.Sesion{}, "", false
	}
	if arg != "" {
		se, ok, err := s.sesionPorTexto(arg)
		if err != nil {
			return fallo()
		}
		if !ok {
			responderError(w, http.StatusNotFound, "La sesión "+arg+" no existe") // nunca cae a otra
			return db.Sesion{}, "", false
		}
		return se, "argumento", true
	}
	activa := ""
	if u := s.pizarra.ultimaSesion(); u != 0 {
		activa = strconv.FormatInt(u, 10)
	}
	for _, c := range []struct{ texto, resolucion string }{{pista, "entorno"}, {activa, "activa"}} {
		if c.texto == "" {
			continue
		}
		se, ok, err := s.sesionPorTexto(c.texto)
		if err != nil {
			return fallo()
		}
		if ok {
			return se, c.resolucion, true
		}
	}
	se, ok, err := db.SesionReciente(s.Base)
	if err != nil {
		return fallo()
	}
	if !ok {
		responderError(w, http.StatusNotFound, "No hay sesiones de Terminal")
		return db.Sesion{}, "", false
	}
	return se, "reciente", true
}

func textoID(id *int64) string {
	if id == nil {
		return ""
	}
	return strconv.FormatInt(*id, 10)
}

// latido: la UI avisa que la pizarra de la sesión está montada y recibe lo
// pendiente: lotes de ops (hasta su ack) y capturas por hacer.
func (s *Servidor) latido(w http.ResponseWriter, r *http.Request) {
	var b struct {
		SesionID int64 `json:"sesion_id"`
	}
	if err := leerJSON(w, r, &b); err != nil {
		responderError(w, 400, "JSON inválido. Manda {\"sesion_id\":N}")
		return
	}
	if _, ok, err := db.ObtenerSesion(s.Base, b.SesionID); err != nil {
		responderError(w, 500, "No se pudo leer la sesión")
		return
	} else if !ok {
		noExiste(w, b.SesionID)
		return
	}
	s.pizarra.latir(b.SesionID)
	pend, err := db.OpsPendientes(s.Base, b.SesionID)
	if err != nil {
		responderError(w, 500, "No se pudieron leer las operaciones")
		return
	}
	ops := make([]map[string]interface{}, 0, len(pend))
	for _, o := range pend {
		ops = append(ops, map[string]interface{}{"id": o.ID, "op": json.RawMessage(o.Op)})
	}
	responder(w, map[string]interface{}{"ops": ops, "capturas": s.pizarra.capturasPara(b.SesionID)})
}

func (s *Servidor) ackOp(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		responderError(w, 404, "Operación no encontrada")
		return
	}
	b, inversa, versiones := leerAck(r)
	if b.IDs == nil {
		b.IDs = []string{}
	}
	resultado := b.Error
	if b.OK {
		j, _ := json.Marshal(map[string][]string{"ids": b.IDs})
		resultado = string(j)
	}
	sesion, op, err := db.ResponderOpHistorial(s.Base, id, b.OK, resultado, inversa, versiones)
	switch {
	case errors.Is(err, db.ErrOpNoPendiente):
		responderError(w, http.StatusConflict, "La operación ya no está pendiente")
		return
	case errors.Is(err, sql.ErrNoRows):
		responderError(w, 404, "Operación no encontrada")
		return
	case err != nil:
		responderError(w, 500, "No se pudo registrar la respuesta")
		return
	}
	if b.OK {
		// Las ops solo las encola un agente (ruta Agente): el evento es suyo aunque el ack venga de la UI.
		detalle := fmt.Sprintf("pizarra_dibujar · %d elementos", len(b.IDs))
		if tipo, n := pizarra.Describir(op); tipo == "marcas" {
			detalle = fmt.Sprintf("pizarra_marcar · %d elementos de marcas", n)
		}
		if err := db.RegistrarEvento(s.Base, db.OrigenAgente, "", db.AccionPizarraEditada, sesion, detalle); err != nil {
			log.Println("No se pudo registrar evento de historial:", err)
		}
	}
	s.pizarra.avisarOp(id, resultadoOp{Aplicada: b.OK, IDs: b.IDs, Error: b.Error})
	responder(w, map[string]bool{"ok": true})
}

func (s *Servidor) encolarOps(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, limiteOps)
	var b struct {
		SesionID *int64       `json:"sesion_id"`
		Pista    string       `json:"pista"`
		Ops      []pizarra.Op `json:"ops"`
	}
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		if esBodyDemasiadoGrande(err) {
			responderError(w, http.StatusRequestEntityTooLarge, "El lote de operaciones es demasiado grande")
			return
		}
		responderError(w, 400, "JSON inválido. Manda {\"ops\":[...]}")
		return
	}
	if err := pizarra.Validar(b.Ops); err != nil {
		responderError(w, 400, err.Error())
		return
	}
	se, resolucion, ok := s.resolverSesion(w, textoID(b.SesionID), b.Pista)
	if !ok {
		return
	}
	s.encolarYEsperar(w, r, se, resolucion, b.Ops)
}

// encolarYEsperar es el cuerpo común de pizarra/ops y pizarra/marcas: exige la
// pizarra montada, encola el lote y espera el ack (la UI lo recoge por latido).
func (s *Servidor) encolarYEsperar(w http.ResponseWriter, r *http.Request, se db.Sesion, resolucion string, ops []pizarra.Op) {
	if !s.pizarra.montada(se.ID) {
		responderError(w, http.StatusConflict, msgPizarraCerrada)
		return
	}
	pizarra.AsignarIDs(ops)
	lote, _ := json.Marshal(ops)
	opID, err := db.EncolarOp(s.Base, se.ID, string(lote))
	if err != nil {
		responderError(w, 500, "No se pudo encolar la operación")
		return
	}
	// Desde acá no hay ninguna conexión de BD en uso: solo se espera en memoria.
	aviso := s.pizarra.esperarOp(opID)
	defer s.pizarra.soltarOp(opID)
	salida := map[string]interface{}{"op_id": opID, "sesion_id": se.ID, "resolucion": resolucion}
	timer := time.NewTimer(espera(s.pizarra.esperaOps, esperaOpsDefecto))
	defer timer.Stop()
	select {
	case res := <-aviso:
		if res.Aplicada {
			salida["estado"], salida["ids"] = "aplicada", res.IDs
		} else {
			salida["estado"], salida["error"] = "fallida", res.Error
		}
	case <-timer.C:
		// El ack pudo llegar antes de esperarOp (aviso perdido): manda la BD.
		salida["estado"], salida["mensaje"] = "pendiente", mensajePendiente
		if estado, resultado, err := db.EstadoOp(s.Base, opID); err == nil {
			completarDesdeBD(salida, estado, resultado)
		}
	case <-r.Context().Done():
		return
	}
	responder(w, salida)
}

// marcarPizarra (pizarra_marcar): valida las marcas, resuelve los objetivos
// contra la escena guardada y las citas contra la carpeta de la sesión, las
// expande a ops (marco + píldora) y las encola como un solo lote (design D3, D7, D8).
func (s *Servidor) marcarPizarra(w http.ResponseWriter, r *http.Request) {
	var b struct {
		SesionID *int64          `json:"sesion_id"`
		Pista    string          `json:"pista"`
		Marcas   []pizarra.Marca `json:"marcas"`
	}
	if err := leerJSON(w, r, &b); err != nil {
		if esBodyDemasiadoGrande(err) {
			responderError(w, http.StatusRequestEntityTooLarge, "El cuerpo de la petición es demasiado grande")
			return
		}
		responderError(w, 400, "JSON inválido. Manda {\"marcas\":[...]}")
		return
	}
	if err := pizarra.ValidarMarcas(b.Marcas); err != nil {
		responderError(w, 400, err.Error())
		return
	}
	se, resolucion, ok := s.resolverSesion(w, textoID(b.SesionID), b.Pista)
	if !ok {
		return
	}
	citas, codigo, msg := s.titulosDeCitas(se, b.Marcas)
	if msg != "" {
		responderError(w, codigo, msg)
		return
	}
	escena, _, _, err := db.ObtenerPizarra(s.Base, se.ID)
	if err != nil {
		responderError(w, 500, "No se pudo leer la pizarra")
		return
	}
	var ids []string
	for _, m := range b.Marcas {
		ids = append(ids, m.Objetivo.IDs...)
	}
	cajas, err := pizarra.Cajas([]byte(escena), ids)
	if err != nil {
		responderError(w, 400, err.Error())
		return
	}
	ops, err := pizarra.ExpandirMarcas(se.ID, b.Marcas, cajas, citas)
	if err != nil {
		responderError(w, 400, err.Error())
		return
	}
	s.encolarYEsperar(w, r, se, resolucion, ops)
}

// titulosDeCitas resuelve cada recurso citado contra la carpeta de la sesión y
// devuelve su título guardado en la BD (el agente nunca lo pone). msg no vacío
// es el motivo del error (400, o 500 si falla la BD).
func (s *Servidor) titulosDeCitas(se db.Sesion, marcas []pizarra.Marca) (map[int64]string, int, string) {
	citas := map[int64]string{}
	for _, m := range marcas {
		if m.Cita == nil {
			continue
		}
		id := m.Cita.RecursoID
		if _, hecho := citas[id]; hecho {
			continue
		}
		if se.CarpetaID == nil {
			return nil, 400, "La sesión no tiene carpeta"
		}
		rec, existe, err := db.ObtenerRecurso(s.Base, id)
		if err != nil {
			return nil, 500, "No se pudo leer el recurso"
		}
		if !existe || rec.CarpetaID != *se.CarpetaID {
			return nil, 400, fmt.Sprintf("El recurso %d no está en la carpeta de la sesión", id)
		}
		citas[id] = rec.Titulo
	}
	return citas, 0, ""
}

// completarDesdeBD vuelca a salida una op ya respondida según la fila guardada.
func completarDesdeBD(salida map[string]interface{}, estado, resultado string) {
	switch estado {
	case "aplicada":
		var r struct {
			IDs []string `json:"ids"`
		}
		json.Unmarshal([]byte(resultado), &r)
		if r.IDs == nil {
			r.IDs = []string{}
		}
		delete(salida, "mensaje")
		salida["estado"], salida["ids"] = "aplicada", r.IDs
	case "fallida":
		delete(salida, "mensaje")
		salida["estado"], salida["error"] = "fallida", resultado
	}
}

func (s *Servidor) pedirCaptura(w http.ResponseWriter, r *http.Request) {
	var b struct {
		SesionID *int64 `json:"sesion_id"`
		Pista    string `json:"pista"`
	}
	if err := leerJSON(w, r, &b); err != nil && err != io.EOF {
		if esBodyDemasiadoGrande(err) {
			responderError(w, http.StatusRequestEntityTooLarge, "El cuerpo de la petición es demasiado grande")
			return
		}
		responderError(w, 400, "JSON inválido")
		return
	}
	se, resolucion, ok := s.resolverSesion(w, textoID(b.SesionID), b.Pista)
	if !ok {
		return
	}
	if !s.pizarra.montada(se.ID) {
		responderError(w, http.StatusConflict, msgTerminalCerrada)
		return
	}
	id, png := s.pizarra.nuevaCaptura(se.ID)
	defer s.pizarra.soltarCaptura(id)
	timer := time.NewTimer(espera(s.pizarra.esperaCaptura, esperaCapturaDefecto))
	defer timer.Stop()
	select {
	case datos := <-png:
		// Content-Type y cabeceras ANTES de escribir el cuerpo (si no, text/plain).
		h := w.Header()
		h.Set("Content-Type", "image/png")
		h.Set("X-Bythos-Sesion", strconv.FormatInt(se.ID, 10))
		h.Set("X-Bythos-Captura", strconv.FormatInt(id, 10))
		h.Set("X-Bythos-Resolucion", resolucion)
		w.Write(datos)
	case <-timer.C:
		responderError(w, http.StatusGatewayTimeout, "La pizarra no devolvió la captura a tiempo: revisa que la pantalla Terminal siga abierta.")
	case <-r.Context().Done():
	}
}

func (s *Servidor) subirCaptura(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || !s.pizarra.hayCaptura(id) {
		responderError(w, 404, "La captura ya no está pendiente")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, limitePNG)
	datos, err := io.ReadAll(r.Body)
	if err != nil {
		if esBodyDemasiadoGrande(err) {
			responderError(w, http.StatusRequestEntityTooLarge, "La captura es demasiado grande (máximo 4MB)")
			return
		}
		responderError(w, 400, "No se pudo leer la captura")
		return
	}
	if !bytes.HasPrefix(datos, firmaPNG) {
		responderError(w, 400, "La captura no es un PNG")
		return
	}
	if !s.pizarra.entregarCaptura(id, datos) {
		responderError(w, 404, "La captura ya no está pendiente")
		return
	}
	responder(w, map[string]bool{"ok": true})
}

// leerPizarraAgente (pizarra_leer): lee la escena GUARDADA, así que funciona
// con la UI cerrada. ?sesion_id= (autoridad) y ?pista= (BYTHOS_SESION).
func (s *Servidor) leerPizarraAgente(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	se, resolucion, ok := s.resolverSesion(w, q.Get("sesion_id"), q.Get("pista"))
	if !ok {
		return
	}
	escena, version, _, err := db.ObtenerPizarra(s.Base, se.ID)
	if err != nil {
		responderError(w, 500, "No se pudo leer la pizarra")
		return
	}
	res, err := pizarra.Resumir([]byte(escena))
	if err != nil {
		responderError(w, 500, "La pizarra guardada está dañada")
		return
	}
	adjuntos, err := adjuntosEnPizarra(s, se.ID, []byte(escena), res.Elementos)
	if err != nil {
		responderError(w, 500, "No se pudieron leer los adjuntos")
		return
	}
	pendientes, err := db.ContarOpsPendientes(s.Base, se.ID)
	if err != nil {
		responderError(w, 500, "No se pudieron leer las operaciones")
		return
	}
	var carpeta interface{}
	if se.CarpetaID != nil {
		carpeta = nombreCarpetaPorID(s, *se.CarpetaID)
	}
	responder(w, map[string]interface{}{
		"sesion":      map[string]interface{}{"id": se.ID, "nombre": se.Nombre, "modo": se.Modo, "carpeta": carpeta},
		"resolucion":  resolucion,
		"version":     version,
		"actualizado": se.Actualizado,
		"elementos":   res.Elementos,
		"resumen":     res.Resumen,
		"limites": map[string]int{
			"elementos": pizarra.MaxElementos, "texto": pizarra.MaxTextoLeido, "ops_por_llamada": pizarra.MaxOps,
		},
		"ops_pendientes": pendientes,
		"truncado":       res.Truncado,
		"adjuntos":       adjuntos,
	})
}

// adjuntoPizarra es un adjunto de la sesión que un elemento vivo de la escena
// guardada referencia. Sin contenido: el agente lo lee con leer_imagen o
// leer_archivo (design D5).
type adjuntoPizarra struct {
	ID        int64    `json:"id"`
	Nombre    string   `json:"nombre"`
	Tipo      string   `json:"tipo"`
	Mime      string   `json:"mime"`
	Tamano    int64    `json:"tamano"`
	Elementos []string `json:"elementos"`
}

// adjuntosEnPizarra cruza los vínculos de la sesión con los elementos vivos de
// la escena y marca en cada elemento de la lista el adjunto al que apunta.
func adjuntosEnPizarra(s *Servidor, sesionID int64, escena []byte, elementos []pizarra.ElementoResumen) ([]adjuntoPizarra, error) {
	refs, err := pizarra.Referencias(escena)
	if err != nil {
		return nil, err
	}
	vinculos, err := db.ListarAdjuntos(s.Base, sesionID)
	if err != nil {
		return nil, err
	}
	salida := []adjuntoPizarra{}
	porSHA := map[string]int64{}
	for _, v := range vinculos {
		if ids := refs[v.SHA256]; len(ids) > 0 {
			porSHA[v.SHA256] = v.ID
			salida = append(salida, adjuntoPizarra{v.ID, v.Nombre, archivos.TipoRecurso(v.Mime), v.Mime, v.Tamano, ids})
		}
	}
	for i := range elementos {
		elementos[i].Adjunto = porSHA[elementos[i].SHA]
	}
	return salida, nil
}

type ackOpBody struct {
	OK          bool            `json:"ok"`
	IDs         []string        `json:"ids"`
	Error       string          `json:"error"`
	Inversa     json.RawMessage `json:"inversa"`
	Versiones   json.RawMessage `json:"versiones"`
	SinDeshacer bool            `json:"sin_deshacer"`
}

// leerAck lee el ack con su propio techo de 2 MiB (no el de 1 MiB de leerJSON:
// rechazarlo dejaría vencer la op). Devuelve también la inversa y las versiones
// a guardar; nil (NULL) si no vienen, si el lote es sin_deshacer o si la
// inversa pasa de 512 KiB. Un body de más de 2 MiB no se puede leer: se toma
// como aplicado sin ids ni inversa, para que la op igual se complete.
func leerAck(r *http.Request) (b ackOpBody, inversa, versiones *string) {
	datos, err := io.ReadAll(io.LimitReader(r.Body, limiteAck+1))
	if err == nil && len(datos) > limiteAck {
		return ackOpBody{OK: true}, nil, nil
	}
	if err := json.Unmarshal(datos, &b); err != nil {
		return ackOpBody{}, nil, nil
	}
	guardar := func(crudo json.RawMessage, tope int) *string {
		if b.SinDeshacer || len(crudo) == 0 || len(crudo) > tope || string(crudo) == "null" {
			return nil
		}
		s := string(crudo)
		return &s
	}
	inversa = guardar(b.Inversa, topeInversa)
	if inversa != nil {
		versiones = guardar(b.Versiones, limiteAck)
	}
	return b, inversa, versiones
}

// listarCambios (GET sesiones/{id}/cambios): los lotes del agente con su
// estado de deshacer, sin la inversa (D17).
func (s *Servidor) listarCambios(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		responderError(w, 404, "Sesión no encontrada")
		return
	}
	if _, ok, err := db.ObtenerSesion(s.Base, id); err != nil {
		responderError(w, 500, "No se pudo leer la sesión")
		return
	} else if !ok {
		noExiste(w, id)
		return
	}
	lista, err := db.ListarCambios(s.Base, id)
	if err != nil {
		responderError(w, 500, "No se pudieron leer los cambios")
		return
	}
	responder(w, lista)
}
