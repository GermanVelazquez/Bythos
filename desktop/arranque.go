package main

// arranque.go — ESPERA ACTIVA al servidor, sin sleeps fijos.
// Antes: time.Sleep(800ms) y cruzar los dedos (Edge a veces llegaba
// a una puerta cerrada, y cuando el server tardaba más, también).
// Ahora: se pregunta GET /api/salud cada 100ms y la ventana abre
// APENAS responde, con techo total de ~10s. Rápido siempre,
// sin importar si la PC vuela o va cargada.

import (
	"fmt"
	"net/http"
	"time"
)

// esperarSalud bloquea hasta que url responde 200 OK o pasa algo malo:
// el servidor murió (errServidor trae su error) o se agota el tiempo total.
// intervalo es cada cuánto preguntar; total, cuánto esperar como máximo.
func esperarSalud(url string, intervalo, total time.Duration, errServidor <-chan error) error {
	limite := time.Now().Add(total)
	for {
		// ¿El servidor ya murió (puerto ocupado, etc.)? No tiene sentido
		// seguir preguntando: devolver su error directo.
		select {
		case err := <-errServidor:
			return fmt.Errorf("el servidor no arrancó: %w", err)
		default:
		}
		resp, err := http.Get(url)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		if !time.Now().Before(limite) {
			return fmt.Errorf("sin respuesta de %s tras %s", url, total)
		}
		time.Sleep(intervalo)
	}
}
