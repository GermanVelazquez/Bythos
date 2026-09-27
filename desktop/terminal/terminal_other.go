//go:build !windows

package terminal

import "syscall"

// sysProcAttrConsola no aplica fuera de Windows (Bythos es una app de
// escritorio para Windows, ver ventana/ con sus llamadas a user32.dll);
// este archivo solo existe para que `go vet`/`go test` sigan compilando
// si alguna vez se corren en otro SO (ej. un editor con gopls en Linux).
func sysProcAttrConsola() *syscall.SysProcAttr { return nil }
