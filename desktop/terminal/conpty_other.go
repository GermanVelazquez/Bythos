//go:build !windows

package terminal

// NuevoPTY no aplica fuera de Windows: ConPTY es una API de Windows. Existe
// para que el paquete siga compilando en otros SO (ver terminal_other.go) y
// para que quien la use reciba un error que sabe traducir (501).
func NuevoPTY(Config) (PTY, error) { return nil, ErrNoSoportado }
