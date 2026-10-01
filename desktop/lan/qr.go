package lan

// qr.go — El QR de emparejamiento: el texto bythos://pair?... (que el
// celular escanea o pega a mano si no hay Play Services) y su render PNG
// como data URI para que la UI lo muestre sin librerías de JS.

import (
	"encoding/base64"
	"net/url"
	"strconv"

	"rsc.io/qr"
)

// QRTexto arma bythos://pair?v=1&h=<ip>&p=<puerto>&k=<spki>&c=<codigo>&n=<pc>.
// El orden de los parámetros es el del diseño; el celular los parsea por clave.
func QRTexto(host string, puerto int, huella, codigo, nombrePC string) string {
	return "bythos://pair?v=1&h=" + url.QueryEscape(host) +
		"&p=" + strconv.Itoa(puerto) +
		"&k=" + url.QueryEscape(huella) +
		"&c=" + url.QueryEscape(codigo) +
		"&n=" + url.QueryEscape(nombrePC)
}

// QRDataURI renderiza texto como PNG (corrección de errores M) en data URI.
func QRDataURI(texto string) (string, error) {
	codigo, err := qr.Encode(texto, qr.M)
	if err != nil {
		return "", err
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(codigo.PNG()), nil
}
