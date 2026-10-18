package lan

import (
	"context"
	"errors"
	"net"
	"testing"
)

func conCategoria(t *testing.T, cat Categoria, err error) {
	t.Helper()
	previa := categoriaRed
	categoriaRed = func(net.IP) (Categoria, error) { return cat, err }
	t.Cleanup(func() { categoriaRed = previa })
}

func TestRedCompuerta(t *testing.T) {
	casos := []struct {
		nombre   string
		cat      Categoria
		err      error
		arranca  bool
		advierte bool
	}{
		{"privada arranca", CategoriaPrivada, nil, true, false},
		{"publica rechaza", CategoriaPublica, nil, false, false},
		{"dominio rechaza", CategoriaDominio, nil, false, false},
		{"desconocida arranca y avisa", CategoriaDesconocida, nil, true, true},
		{"error de deteccion arranca y avisa", "", errors.New("com roto"), true, true},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			conCategoria(t, c.cat, c.err)
			rc := NuevoReceptor()
			rc.Puerto = 0
			conCarpetaLANTemporal(t)
			err := rc.Start(ifazLoopback("127.0.0.1/32"))
			defer rc.Stop(context.Background())
			if c.arranca {
				if err != nil || !rc.Activo() {
					t.Fatalf("debía arrancar: err=%v activo=%v", err, rc.Activo())
				}
			} else if !errors.Is(err, ErrRedNoPrivada) || rc.Activo() {
				t.Fatalf("debía rechazar con ErrRedNoPrivada: err=%v activo=%v", err, rc.Activo())
			}
			if rc.RedDesconocida() != c.advierte {
				t.Fatalf("RedDesconocida = %v, quería %v", rc.RedDesconocida(), c.advierte)
			}
		})
	}
}
