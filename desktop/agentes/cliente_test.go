package agentes

// cliente_test.go — cuando Bythos NO está corriendo, cualquier tool debe
// devolver un tool error con el mensaje en español, no un panic ni un
// error crudo de red en inglés.

import (
	"net"
	"net/http"
	"testing"
	"time"
)

// clientePuertoLibre apunta a un puerto que nadie escucha: simula
// "la app no está abierta" sin depender de que un puerto real quede libre
// en la máquina de CI (buscamos uno, lo cerramos, y usamos esa dirección).
func clientePuertoLibre(t *testing.T) *Cliente {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("no se pudo reservar un puerto: %v", err)
	}
	addr := l.Addr().String()
	l.Close() // liberado: ahora nadie escucha ahí

	return &Cliente{
		BaseURL: "http://" + addr,
		Host:    "localhost:8080",
		HTTP:    &http.Client{Timeout: 2 * time.Second},
	}
}

func TestAppCerradaDevuelveErrorAmigable(t *testing.T) {
	c := clientePuertoLibre(t)
	sesion := sesionDePrueba(t, c)

	isErr, msg := llamar(t, sesion, "listar_carpetas", nil, nil)
	if !isErr {
		t.Fatalf("esperaba tool error con la app cerrada")
	}
	if msg != mensajeAppCerrada {
		t.Fatalf("mensaje inesperado: %q, esperaba %q", msg, mensajeAppCerrada)
	}
}

func TestAppCerradaEnHerramientaDeEscritura(t *testing.T) {
	c := clientePuertoLibre(t)
	sesion := sesionDePrueba(t, c)

	isErr, msg := llamar(t, sesion, "crear_carpeta", map[string]any{"nombre": "X"}, nil)
	if !isErr {
		t.Fatalf("esperaba tool error con la app cerrada")
	}
	if msg != mensajeAppCerrada {
		t.Fatalf("mensaje inesperado: %q, esperaba %q", msg, mensajeAppCerrada)
	}
}
