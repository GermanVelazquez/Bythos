//go:build windows

package ventana

// ventana_windows.go — El diálogo nativo de Windows (MessageBoxW): Go puro,
// sin CGO, user32.dll ya está en todo Windows.

import (
	"syscall"
	"unsafe"
)

// mostrarMensajeNativo llama a MessageBoxW con el estilo pedido.
func mostrarMensajeNativo(titulo, mensaje string, estilo uint32) {
	t, _ := syscall.UTF16PtrFromString(titulo)
	m, _ := syscall.UTF16PtrFromString(mensaje)
	user32 := syscall.NewLazyDLL("user32.dll")
	mensajeBox := user32.NewProc("MessageBoxW")
	mensajeBox.Call(0, // sin ventana dueña: el diálogo va al frente
		uintptr(unsafe.Pointer(m)),
		uintptr(unsafe.Pointer(t)),
		uintptr(estilo))
}
