package lan

// red.go — Requirement "Windows Network Profile": el listener solo arranca
// si la red de la interfaz bindeada es Privada. La detección real vive en
// red_windows.go (COM INetworkListManager); acá el tipo y el seam de test.

import (
	"errors"
	"net"
)

// Categoria es el perfil de red de Windows de la interfaz bindeada.
type Categoria string

const (
	CategoriaPrivada     Categoria = "privada"
	CategoriaPublica     Categoria = "publica"
	CategoriaDominio     Categoria = "dominio"
	CategoriaDesconocida Categoria = "desconocida"
)

// ErrRedNoPrivada: Start sobre una red Pública o de Dominio. La API de
// control (unidad 5a) lo mapea al código de wire `red_no_privada`.
var ErrRedNoPrivada = errors.New("red_no_privada")

// categoriaRed es el seam de test; el default consulta al SO.
var categoriaRed = categoriaRedSO

// categoriaDe nunca falla hacia "privada": si no se puede determinar
// (error o IP sin red asociada) devuelve desconocida, que arranca pero
// avisa en la UI.
func categoriaDe(ip net.IP) Categoria {
	cat, err := categoriaRed(ip)
	if err != nil || cat == "" {
		return CategoriaDesconocida
	}
	return cat
}
