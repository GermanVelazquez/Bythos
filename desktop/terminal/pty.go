package terminal

import (
	"errors"
	"io"
)

// ErrNoSoportado indica que este sistema no puede abrir una terminal
// embebida: no es Windows, o es un Windows anterior a la compilación 17763
// (Windows 10 1809), la primera que trae ConPTY. La API lo traduce a un 501
// y la UI ofrece "Abrir agente" (la terminal externa de siempre).
var ErrNoSoportado = errors.New("terminal: la terminal embebida no está soportada en este sistema")

// Config describe el proceso que corre dentro de un PTY. Igual que en el
// Lanzador, el comando y sus args los arma Go con valores fijos (ver
// design D2): nada de esto viaja por HTTP ni lo escribe un usuario.
type Config struct {
	// Comando es la ruta del ejecutable (en la práctica %ComSpec%).
	Comando string
	// Args son los argumentos, SIN incluir el ejecutable.
	Args []string
	// Dir es el directorio de trabajo; vacío hereda el del proceso.
	Dir string
	// Env son entradas "CLAVE=valor"; nil hereda el entorno del proceso.
	Env []string
	// Cols y Filas son el tamaño inicial; 0 usa 80x24.
	Cols, Filas uint16
}

// tamano devuelve el tamaño inicial con los valores por defecto aplicados.
func (c Config) tamano() (cols, filas uint16) {
	cols, filas = c.Cols, c.Filas
	if cols == 0 {
		cols = 80
	}
	if filas == 0 {
		filas = 24
	}
	return cols, filas
}

// PTY es una pseudo-terminal con un proceso hijo ya arrancado. Read entrega
// la salida (flujo UTF-8 con secuencias VT) y Write la entrada del usuario.
//
// Quien llama DEBE leer Read en una goroutine propia hasta EOF: si nadie
// vacía la salida, ConPTY se bloquea. Cerrar es idempotente y mata TODO el
// árbol de procesos del hijo.
type PTY interface {
	io.Reader
	io.Writer
	// Redimensionar cambia el tamaño de la terminal.
	Redimensionar(cols, filas uint16) error
	// Fin entrega el código de salida del proceso hijo cuando termina y
	// después se cierra. Que el hijo termine NO cierra el PTY: hay que
	// llamar a Cerrar.
	Fin() <-chan int
	// Cerrar mata el árbol de procesos y libera todo. Seguro de llamar
	// varias veces y desde varias goroutines.
	Cerrar() error
}
