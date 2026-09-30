package lan

// guardia.go — Requirement "Private Network Enforcement": la IP remota
// tiene que caer dentro de la subred de la interfaz bindeada, no alcanza
// con ser RFC1918. Rechazo = 403 sin tocar el handler de negocio.

import (
	"encoding/json"
	"net"
	"net/http"
)

// remotoPermitido es el seam de test: un test puede pisarlo para simular
// un remoto "dentro de subred" sin dos IPs de verdad (ej. 127.0.0.1).
var remotoPermitido = subredPermite

// subredPermite exige la MISMA subred que la interfaz bindeada, igual
// que la regla del firewall del instalador (remoteip=localsubnet).
func subredPermite(remoto net.IP, rango *net.IPNet) bool {
	if remoto == nil || rango == nil {
		return false
	}
	r4 := remoto.To4()
	if r4 == nil {
		return false
	}
	return rango.Contains(r4)
}

// conGuardia rechaza cualquier request cuyo remoto no pase
// remotoPermitido; next nunca se llama en ese caso.
func conGuardia(rango *net.IPNet, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			host = r.RemoteAddr
		}
		if !remotoPermitido(net.ParseIP(host), rango) {
			responderErrorLAN(w, http.StatusForbidden, "origen_no_permitido", "Origen no permitido")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// responderErrorLAN escribe {"error","mensaje"} (wire protocol LAN).
// Implementación mínima acá; la unidad 3 (respuestas.go) trae la tabla
// completa de códigos y puede reemplazar/reusar esta función.
func responderErrorLAN(w http.ResponseWriter, codigo int, codigoErr, mensaje string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(codigo)
	json.NewEncoder(w).Encode(map[string]string{"error": codigoErr, "mensaje": mensaje})
}
