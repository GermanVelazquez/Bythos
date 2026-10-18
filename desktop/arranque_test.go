package main

// arranque_test.go — El polling abre APENAS el server responde.
// Se prueba contra httptest (server real en localhost efímero),
// con tiempos chicos para que la suite siga en segundos.

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"
)

const (
	intervaloTest = 5 * time.Millisecond
	rapidoTest    = 200 * time.Millisecond
)

// nadieMuere es un errServidor que nunca entrega nada.
func nadieMuere() <-chan error { return make(chan error) }

func TestEsperarSalud(t *testing.T) {
	casos := []struct {
		nombre      string
		responder   func(contador *atomic.Int32) http.HandlerFunc
		sinServidor bool // no levanta httptest: pregunta a un puerto cerrado
		errServidor <-chan error
		total       time.Duration
		quiereError bool
	}{
		{
			nombre: "responde ya: vuelve al instante",
			responder: func(_ *atomic.Int32) http.HandlerFunc {
				return respuestaSalud(os.Getpid(), "1.0.0")
			},
			errServidor: nadieMuere(),
			total:       rapidoTest,
		},
		{
			nombre: "tarda unos intentos: espera y abre",
			responder: func(contador *atomic.Int32) http.HandlerFunc {
				return func(w http.ResponseWriter, r *http.Request) {
					// Las primeras 3 preguntas fallan (server tibio),
					// después responde: el polling debe aguantar.
					if contador.Add(1) <= 3 {
						w.WriteHeader(http.StatusServiceUnavailable)
						return
					}
					respuestaSalud(os.Getpid(), "1.0.0")(w, r)
				}
			},
			errServidor: nadieMuere(),
			total:       rapidoTest,
		},
		{
			nombre: "nunca responde: agota el tiempo",
			responder: func(_ *atomic.Int32) http.HandlerFunc {
				return func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(http.StatusServiceUnavailable)
				}
			},
			errServidor: nadieMuere(),
			total:       50 * time.Millisecond,
			quiereError: true,
		},
		{
			nombre: "puerto caído: agota el tiempo",
			responder: func(_ *atomic.Int32) http.HandlerFunc {
				return func(w http.ResponseWriter, _ *http.Request) {}
			},
			sinServidor: true,
			errServidor: nadieMuere(),
			total:       30 * time.Millisecond,
			quiereError: true,
		},
		{
			nombre: "el servidor murió: devuelve su error sin esperar el techo",
			responder: func(_ *atomic.Int32) http.HandlerFunc {
				return func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(http.StatusServiceUnavailable)
				}
			},
			errServidor: func() <-chan error {
				ch := make(chan error, 1)
				ch <- fmt.Errorf("puerto ocupado")
				return ch
			}(),
			total:       5 * time.Second,
			quiereError: true,
		},
	}
	for _, tt := range casos {
		t.Run(tt.nombre, func(t *testing.T) {
			var contador atomic.Int32
			url := ""
			if tt.sinServidor {
				// Puerto casi seguro cerrado: nada escucha acá.
				url = "http://localhost:9/salud"
			} else {
				srv := httptest.NewServer(tt.responder(&contador))
				defer srv.Close()
				url = srv.URL
			}
			inicio := time.Now()
			err := esperarSalud(url, intervaloTest, tt.total, tt.errServidor)
			tardo := time.Since(inicio)
			if tt.quiereError && err == nil {
				t.Fatal("se esperaba error, fue nil")
			}
			if !tt.quiereError && err != nil {
				t.Fatalf("se esperaba éxito, fue: %v", err)
			}
			// El camino feliz nunca tarda el techo: abre APENAS responde.
			if !tt.quiereError && tardo >= tt.total {
				t.Fatalf("debió volver antes del techo %s, tardó %s", tt.total, tardo)
			}
		})
	}
}

// respuestaSalud imita /api/salud: 200 con pid y versión.
func respuestaSalud(pid int, version string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, `{"ok":true,"version":%q,"pid":%d}`, version, pid)
	}
}

// Un 200 de OTRO proceso (pid ajeno, o sin pid) NO es nuestra salud:
// es el servidor de una instancia vieja que todavía tenía el puerto.
func TestEsperarSaludRechazaPidAjeno(t *testing.T) {
	casos := []struct {
		nombre  string
		handler http.HandlerFunc
	}{
		{"pid distinto", respuestaSalud(os.Getpid()+1, "1.0.0")},
		{"sin pid", func(w http.ResponseWriter, _ *http.Request) { fmt.Fprint(w, `{"ok":true}`) }},
		{"cuerpo no JSON", func(w http.ResponseWriter, _ *http.Request) { fmt.Fprint(w, "hola") }},
	}
	for _, tt := range casos {
		t.Run(tt.nombre, func(t *testing.T) {
			srv := httptest.NewServer(tt.handler)
			defer srv.Close()
			if err := esperarSalud(srv.URL, intervaloTest, 50*time.Millisecond, nadieMuere()); err == nil {
				t.Fatal("debía rechazar una salud que no es de este proceso")
			}
		})
	}
}

func TestSondearInstancia(t *testing.T) {
	casos := []struct {
		nombre  string
		handler http.HandlerFunc // nil = nada escucha
		quiere  resultadoSonda
	}{
		{"misma versión", respuestaSalud(4242, "1.1.0"), instanciaMisma},
		{"otra versión", respuestaSalud(4242, "1.0.0"), instanciaOtraVersion},
		{"sin respuesta", nil, instanciaSinRespuesta},
		{"responde sin versión", func(w http.ResponseWriter, _ *http.Request) { fmt.Fprint(w, "x") }, instanciaSinRespuesta},
	}
	for _, tt := range casos {
		t.Run(tt.nombre, func(t *testing.T) {
			url := "http://localhost:9/salud"
			if tt.handler != nil {
				srv := httptest.NewServer(tt.handler)
				defer srv.Close()
				url = srv.URL
			}
			if got := sondearInstancia(url, "1.1.0", intervaloTest, 50*time.Millisecond); got != tt.quiere {
				t.Fatalf("sondearInstancia = %v, quería %v", got, tt.quiere)
			}
		})
	}
}

func TestSegundoLanzamiento(t *testing.T) {
	casos := []struct {
		nombre      string
		handler     http.HandlerFunc // nil = nada escucha
		abreError   error
		quiereCod   int
		quiereAbre  bool
		quiereAviso string
	}{
		{"misma versión: abre la ventana y sale 0", respuestaSalud(1, "1.1.0"), nil, 0, true, ""},
		{"misma versión y la ventana falla: sale 1", respuestaSalud(1, "1.1.0"), fmt.Errorf("sin Edge"), 1, true, ""},
		{"otra versión: avisa y sale 1", respuestaSalud(1, "0.9.0"), nil, 1, false, msgOtraVersion},
		{"sin respuesta: avisa y sale 1", nil, nil, 1, false, msgSinRespuesta},
	}
	for _, tt := range casos {
		t.Run(tt.nombre, func(t *testing.T) {
			url := "http://localhost:9/salud"
			if tt.handler != nil {
				srv := httptest.NewServer(tt.handler)
				defer srv.Close()
				url = srv.URL
			}
			abrio, aviso := false, ""
			cod := segundoLanzamiento(url, "http://localhost:8080", "1.1.0", intervaloTest, 50*time.Millisecond,
				func(string) error { abrio = true; return tt.abreError },
				func(_, m string) { aviso = m })
			if cod != tt.quiereCod || abrio != tt.quiereAbre || aviso != tt.quiereAviso {
				t.Fatalf("cod=%d abrio=%v aviso=%q; quería cod=%d abrio=%v aviso=%q",
					cod, abrio, aviso, tt.quiereCod, tt.quiereAbre, tt.quiereAviso)
			}
		})
	}
}
