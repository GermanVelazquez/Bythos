package api

// receptor.go — API de control del receptor LAN (POST /api/receptor/...).
// La UI de la PC (ventana de Bythos) enciende/apaga el receptor, muestra el
// QR y aprueba o rechaza celulares. lan define los puertos y api los usa;
// lan nunca importa api.
//
// Más estricto que conGuardia, igual que agente.go: Origin EXACTO de la
// ventana. La extensión jamás debe poder leer el código de emparejamiento
// ni encender el puerto LAN. Por eso incluso el estado es POST: un GET
// simple cross-site no lleva un Origin que podamos exigir tan limpio.

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"strconv"
	"time"

	"bythos-desktop/db"
	"bythos-desktop/lan"
)

// receptorLAN es el pedazo de lan.Receptor que usa la API de control;
// existe para que los tests inyecten uno falso sin abrir puertos TLS.
type receptorLAN interface {
	Start(iface lan.Interfaz) error
	Stop(ctx context.Context) error
	Activo() bool
	RedDesconocida() bool
	Direccion() string
	Huella() (string, error)
}

// ControlReceptor reúne el receptor LAN y sus piezas (emparejador, almacén
// de subidas, servicio de links/subidas) detrás de la API de control.
type ControlReceptor struct {
	s          *Servidor
	rc         receptorLAN
	emp        *lan.Emparejador
	almacen    *lan.AlmacenSubidas
	servicio   *lan.Servicio
	interfaces func() ([]lan.Interfaz, error)
	nombrePC   func() (string, error)
	parar      chan struct{}
}

// NuevoControlReceptor arma el receptor completo (apagado por defecto):
// registra las rutas /v1 en el mux del receptor ANTES de cualquier Start y
// lanza la limpieza periódica de subidas viejas.
func NuevoControlReceptor(s *Servidor) *ControlReceptor {
	rc := lan.NuevoReceptor()
	emp := lan.NuevoEmparejador(s.Base, rc.Reloj)
	almacen := lan.NuevoAlmacenSubidas(rc.Reloj)
	servicio := lan.NuevoServicio(s.Base, NuevoPuenteLAN(s), almacen)
	emp.Registrar(rc.Mux)
	servicio.Registrar(rc.Mux)

	c := &ControlReceptor{
		s: s, rc: rc, emp: emp, almacen: almacen, servicio: servicio,
		interfaces: lan.InterfacesDisponibles, nombrePC: os.Hostname,
		parar: make(chan struct{}),
	}
	go almacen.Vigilar(c.parar, time.Hour)
	return c
}

// Cerrar apaga el receptor (disparador "salida de la app"): Stop con gracia,
// corta la limpieza y espera a los finalizadores de subidas en vuelo.
func (c *ControlReceptor) Cerrar() {
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	_ = c.rc.Stop(ctx)
	close(c.parar)
	c.servicio.Esperar()
}

// montar registra las rutas de control en el mux de la API de la UI.
func (c *ControlReceptor) montar(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/receptor/estado", c.conOrigen(c.estado))
	mux.HandleFunc("POST /api/receptor/activar", c.conOrigen(c.activar))
	mux.HandleFunc("POST /api/receptor/desactivar", c.conOrigen(c.desactivar))
	mux.HandleFunc("POST /api/receptor/codigo/renovar", c.conOrigen(c.renovarCodigo))
	mux.HandleFunc("POST /api/receptor/solicitudes/{id}/aprobar", c.conOrigen(c.aprobar))
	mux.HandleFunc("POST /api/receptor/solicitudes/{id}/rechazar", c.conOrigen(c.rechazar))
	mux.HandleFunc("DELETE /api/receptor/dispositivos/{id}", c.conOrigen(c.revocar))
}

// conOrigen exige el Origin EXACTO de la ventana (Vite solo con BYTHOS_DEV=1);
// sin Origin o con uno distinto responde 403.
func (c *ControlReceptor) conOrigen(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if !origenesAgentePermitidos[origin] && !(modoDev() && origin == origenVite) {
			responderError(w, http.StatusForbidden, "Esta acción solo se puede pedir desde la ventana de Bythos")
			return
		}
		h(w, r)
	}
}

// errorControl responde {"error": código, "mensaje": ...}; el código es el
// contrato con la UI (5b), el mensaje es para humanos.
func errorControl(w http.ResponseWriter, status int, codigo, mensaje string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	responder(w, map[string]string{"error": codigo, "mensaje": mensaje})
}

func (c *ControlReceptor) estado(w http.ResponseWriter, r *http.Request) {
	responder(w, c.armarEstado())
}

func (c *ControlReceptor) armarEstado() map[string]any {
	activo := c.rc.Activo()
	est := map[string]any{
		"activo": activo, "ip": "", "puerto": 0, "categoria_red": "",
		"interfaces": []any{}, "qr_png": "", "qr_texto": "", "codigo_expira": "",
		"solicitudes": c.emp.Pendientes(), "dispositivos": []any{},
		"transferencias": []any{}, "ultima_conexion": "",
	}

	if ifs, err := c.interfaces(); err == nil {
		lista := []map[string]string{}
		for _, i := range ifs {
			lista = append(lista, map[string]string{
				"ip": i.IP.String(), "nombre": i.Nombre, "categoria": string(lan.CategoriaDeIP(i.IP)),
			})
		}
		est["interfaces"] = lista
	}

	ultima := ""
	devs := []map[string]any{}
	if ds, err := db.ListarDispositivos(c.s.Base); err == nil {
		for _, d := range ds {
			if d.RevocadoEn != "" {
				continue
			}
			devs = append(devs, map[string]any{
				"id": d.ID, "nombre": d.Nombre, "creado_en": d.CreadoEn, "ultimo_uso": d.UltimoUso,
			})
			if d.UltimoUso > ultima {
				ultima = d.UltimoUso
			}
		}
	}
	est["dispositivos"], est["ultima_conexion"] = devs, ultima

	trans := []map[string]any{}
	for _, t := range c.almacen.ListarConsulta() {
		trans = append(trans, map[string]any{
			"id": t.ID, "dispositivo_id": t.DispositivoID, "nombre": t.Nombre,
			"tamano": t.Tamano, "offset": t.Offset, "estado": t.Estado,
		})
	}
	est["transferencias"] = trans

	if !activo {
		return est
	}
	host, puertoTxt, err := net.SplitHostPort(c.rc.Direccion())
	if err != nil {
		return est
	}
	puerto, _ := strconv.Atoi(puertoTxt)
	est["ip"], est["puerto"] = host, puerto
	if c.rc.RedDesconocida() {
		est["categoria_red"] = string(lan.CategoriaDesconocida)
	} else {
		est["categoria_red"] = string(lan.CategoriaPrivada)
	}
	huella, err := c.rc.Huella()
	if err != nil {
		return est
	}
	codigo, expira := c.emp.Codigo()
	nombre, _ := c.nombrePC()
	texto := lan.QRTexto(host, puerto, huella, codigo, nombre)
	est["qr_texto"], est["codigo_expira"] = texto, expira.Format(time.RFC3339)
	if png, err := lan.QRDataURI(texto); err == nil {
		est["qr_png"] = png
	}
	return est
}

func (c *ControlReceptor) activar(w http.ResponseWriter, r *http.Request) {
	var body struct {
		IP string `json:"ip"`
	}
	if r.ContentLength != 0 { // cuerpo vacío = "elegí la mejor interfaz"
		if err := leerJSON(w, r, &body); err != nil {
			errorControl(w, http.StatusBadRequest, "cuerpo_invalido", "JSON inválido")
			return
		}
	}
	ifs, err := c.interfaces()
	if err != nil {
		errorControl(w, http.StatusInternalServerError, "error_interno", "No se pudieron leer las redes")
		return
	}
	var elegida *lan.Interfaz
	for i := range ifs {
		if body.IP == "" || ifs[i].IP.String() == body.IP {
			elegida = &ifs[i]
			break
		}
	}
	if elegida == nil {
		errorControl(w, http.StatusConflict, "sin_interfaz", "No hay una red privada disponible")
		return
	}
	if err := c.rc.Start(*elegida); err != nil {
		switch {
		case errors.Is(err, lan.ErrRedNoPrivada):
			errorControl(w, http.StatusServiceUnavailable, lan.ErrRedNoPrivadaCable, "La red no es Privada")
		case errors.Is(err, lan.ErrPuertoOcupado):
			errorControl(w, http.StatusConflict, "puerto_ocupado", "El puerto del receptor ya está en uso")
		default:
			errorControl(w, http.StatusInternalServerError, "error_interno", "No se pudo activar el receptor")
		}
		return
	}
	responder(w, c.armarEstado())
}

// desactivar no lee el body: la UI lo manda con fetch keepalive al cerrar
// la pantalla (pagehide) y ahí el body puede no llegar. Idempotente.
func (c *ControlReceptor) desactivar(w http.ResponseWriter, r *http.Request) {
	if err := c.rc.Stop(r.Context()); err != nil {
		errorControl(w, http.StatusInternalServerError, "error_interno", "No se pudo apagar el receptor")
		return
	}
	responder(w, map[string]bool{"ok": true})
}

func (c *ControlReceptor) renovarCodigo(w http.ResponseWriter, r *http.Request) {
	c.emp.Renovar()
	responder(w, c.armarEstado())
}

func (c *ControlReceptor) aprobar(w http.ResponseWriter, r *http.Request) {
	if _, err := c.emp.Aprobar(r.PathValue("id")); err != nil {
		c.errorSolicitud(w, err)
		return
	}
	responder(w, map[string]bool{"ok": true})
}

func (c *ControlReceptor) rechazar(w http.ResponseWriter, r *http.Request) {
	if err := c.emp.Rechazar(r.PathValue("id")); err != nil {
		c.errorSolicitud(w, err)
		return
	}
	responder(w, map[string]bool{"ok": true})
}

func (c *ControlReceptor) errorSolicitud(w http.ResponseWriter, err error) {
	if errors.Is(err, lan.ErrSolicitudNoEncontrada) {
		errorControl(w, http.StatusNotFound, "solicitud_no_encontrada", "La solicitud no existe o ya se resolvió")
		return
	}
	errorControl(w, http.StatusInternalServerError, "error_interno", "No se pudo resolver la solicitud")
}

// revocar corta al celular de inmediato (el token deja de resolver) y borra
// sus subidas a medias del almacén.
func (c *ControlReceptor) revocar(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		errorControl(w, http.StatusBadRequest, "id_invalido", "Id de dispositivo inválido")
		return
	}
	if err := db.RevocarDispositivo(c.s.Base, id); err != nil {
		errorControl(w, http.StatusInternalServerError, "error_interno", "No se pudo revocar el dispositivo")
		return
	}
	c.almacen.EliminarDispositivo(id)
	responder(w, map[string]bool{"ok": true})
}
