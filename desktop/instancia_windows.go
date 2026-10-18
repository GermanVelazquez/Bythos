//go:build windows

package main

// instancia_windows.go — UNA sola instancia por sesión de Windows.
// Un mutex con nombre ("Local\" = espacio de nombres de la sesión, el
// mismo que usa el instalador con AppMutex) marca "Bythos está abierto".
// Windows lo libera solo si el proceso muere (crash incluido), así que
// no puede quedar un candado viejo: el estado "stale" no existe.

import (
	"syscall"
	"unsafe"
)

const (
	nombreMutexInstancia = `Local\Bythos.Instancia`
	errorYaExiste        = syscall.Errno(183) // ERROR_ALREADY_EXISTS
)

// adquirirInstancia toma el mutex de Bythos. yaExiste=true significa que
// OTRO proceso ya lo tenía (esta es una segunda instancia).
func adquirirInstancia() (liberar func(), yaExiste bool, err error) {
	return adquirirMutex(nombreMutexInstancia)
}

// adquirirMutex es la pieza genérica (el nombre se parametriza para que
// los tests no choquen con un Bythos real abierto).
func adquirirMutex(nombre string) (liberar func(), yaExiste bool, err error) {
	n, err := syscall.UTF16PtrFromString(nombre)
	if err != nil {
		return nil, false, err
	}
	crear := syscall.NewLazyDLL("kernel32.dll").NewProc("CreateMutexW")
	h, _, errno := crear.Call(0, 0, uintptr(unsafe.Pointer(n)))
	if h == 0 {
		return nil, false, errno
	}
	liberar = func() { syscall.CloseHandle(syscall.Handle(h)) }
	// CreateMutexW devuelve un handle válido incluso si ya existía:
	// la diferencia solo viene en GetLastError.
	return liberar, errno == errorYaExiste, nil
}
