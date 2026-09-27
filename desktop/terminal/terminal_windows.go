//go:build windows

package terminal

import "syscall"

// createNewConsole (CREATE_NEW_CONSOLE, winbase.h) fuerza una consola
// visible nueva para el proceso hijo. Bythos instalado corre con
// -H=windowsgui (sin consola propia): un cmd.exe lanzado desde ahí SUELE
// recibir consola nueva automáticamente porque el padre no tiene una,
// pero eso es un detalle de implementación de Windows, no una garantía
// documentada. Pedirla explícito evita depender de ese comportamiento.
const createNewConsole = 0x00000010

func sysProcAttrConsola() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{CreationFlags: createNewConsole}
}
