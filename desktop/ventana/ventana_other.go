//go:build !windows

package ventana

// ventana_other.go — Fuera de Windows no hay MessageBoxW: Bythos de
// escritorio solo se distribuye para Windows, así que el aviso va a stderr
// (alcanza para que el paquete compile y se pruebe en Linux).

import (
	"fmt"
	"os"
)

func mostrarMensajeNativo(titulo, mensaje string, _ uint32) {
	fmt.Fprintf(os.Stderr, "%s: %s\n", titulo, mensaje)
}
