package lan

// respuestas.go — El sobre de error del protocolo LAN ({"error","mensaje"})
// y la tabla completa de códigos de cable del diseño (Wire Protocol). Los
// códigos son contrato con el celular: no se renombran.

import (
	"encoding/json"
	"net/http"
)

// Códigos de error de cable.
const (
	ErrTokenInvalido       = "token_invalido"
	ErrOrigenNoPermitido   = "origen_no_permitido"
	ErrCodigoInvalido      = "codigo_invalido"
	ErrCodigoExpirado      = "codigo_expirado"
	ErrBloqueado           = "bloqueado"
	ErrCarpetaNoEncontrada = "carpeta_no_encontrada"
	ErrSubidaNoEncontrada  = "subida_no_encontrada"
	ErrOffsetInvalido      = "offset_invalido"
	ErrSubidaOcupada       = "subida_ocupada"
	ErrTamanoExcedido      = "tamano_excedido"
	ErrHashParteInvalido   = "hash_parte_invalido"
	ErrTipoNoPermitido     = "tipo_no_permitido"
	ErrDemasiadasSubidas   = "demasiadas_subidas"
	ErrCuerpoInvalido      = "cuerpo_invalido"
	ErrRedNoPrivadaCable   = "red_no_privada" // ErrRedNoPrivada (red.go) en el cable; lo mapea la API de control (5a)
)

// errorCable asocia cada código con su status HTTP y su mensaje en español.
var errorCable = map[string]struct {
	Status  int
	Mensaje string
}{
	ErrTokenInvalido:       {http.StatusUnauthorized, "Token inválido o dispositivo no emparejado"},
	ErrOrigenNoPermitido:   {http.StatusForbidden, "Origen no permitido"},
	ErrCodigoInvalido:      {http.StatusForbidden, "Código de emparejamiento inválido"},
	ErrCodigoExpirado:      {http.StatusGone, "El código expiró: volvé a escanear el QR"},
	ErrBloqueado:           {http.StatusTooManyRequests, "Demasiados intentos: esperá un momento"},
	ErrCarpetaNoEncontrada: {http.StatusNotFound, "La carpeta no existe"},
	ErrSubidaNoEncontrada:  {http.StatusNotFound, "La subida no existe"},
	ErrOffsetInvalido:      {http.StatusConflict, "El offset no coincide con lo recibido"},
	ErrSubidaOcupada:       {http.StatusConflict, "La subida está ocupada"},
	ErrTamanoExcedido:      {http.StatusRequestEntityTooLarge, "El archivo excede el tamaño máximo"},
	ErrHashParteInvalido:   {http.StatusUnprocessableEntity, "El hash de la parte no coincide"},
	ErrTipoNoPermitido:     {http.StatusUnsupportedMediaType, "Tipo de archivo no permitido"},
	ErrDemasiadasSubidas:   {http.StatusTooManyRequests, "Demasiadas subidas activas"},
	ErrCuerpoInvalido:      {http.StatusBadRequest, "Cuerpo de la request inválido"},
	ErrRedNoPrivadaCable:   {http.StatusServiceUnavailable, "La red de la PC no está marcada como Privada"},
}

// responderError escribe el sobre para un código de la tabla.
func responderError(w http.ResponseWriter, codigo string) {
	e, ok := errorCable[codigo]
	if !ok {
		e.Status, e.Mensaje = http.StatusInternalServerError, "Error interno"
	}
	responderErrorLAN(w, e.Status, codigo, e.Mensaje)
}

// responderErrorLAN escribe {"error","mensaje"} con status explícito.
func responderErrorLAN(w http.ResponseWriter, status int, codigo, mensaje string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": codigo, "mensaje": mensaje})
}

// responderJSON escribe un cuerpo JSON de éxito.
func responderJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
