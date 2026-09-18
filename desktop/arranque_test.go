package main

// arranque_test.go — El polling abre APENAS el server responde.
// Se prueba contra httptest (server real en localhost efímero),
// con tiempos chicos para que la suite siga en segundos.

import (
	"fmt"
	"net/http"
	"net/http/httptest"
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
				return func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }
			},
			errServidor: nadieMuere(),
			total:       rapidoTest,
		},
		{
			nombre: "tarda unos intentos: espera y abre",
			responder: func(contador *atomic.Int32) http.HandlerFunc {
				return func(w http.ResponseWriter, _ *http.Request) {
					// Las primeras 3 preguntas fallan (server tibio),
					// después responde: el polling debe aguantar.
					if contador.Add(1) <= 3 {
						w.WriteHeader(http.StatusServiceUnavailable)
						return
					}
					w.WriteHeader(http.StatusOK)
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
