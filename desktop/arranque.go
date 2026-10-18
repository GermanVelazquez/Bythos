package main

// arranque.go — ESPERA ACTIVA al servidor, sin sleeps fijos.
// Antes: time.Sleep(800ms) y cruzar los dedos (Edge a veces llegaba
// a una puerta cerrada, y cuando el server tardaba más, también).
// Ahora: se pregunta GET /api/salud cada 100ms y la ventana abre
// APENAS responde, con techo total de ~10s. Rápido siempre,
// sin importar si la PC vuela o va cargada.
//
// Además: UNA sola instancia. Un 200 de /api/salud solo vale si trae
// NUESTRO pid; si no, es el servidor de otra instancia que tenía el
// puerto (ver instancia_*.go y el segundo lanzamiento al final).

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"
)

// Textos del segundo lanzamiento (diálogos nativos).
const (
	msgOtraVersion  = "Hay otra versión de Bythos abierta; ciérrala e inténtalo de nuevo."
	msgSinRespuesta = "Bythos ya está abierto pero no responde."
)

// saludRespuesta es lo que nos interesa del cuerpo de /api/salud.
type saludRespuesta struct {
	Version string `json:"version"`
	Pid     int    `json:"pid"`
}

// leerSalud pregunta una vez a url. ok=false si no hubo 200 o el cuerpo
// no es el JSON de salud de Bythos.
func leerSalud(url string) (saludRespuesta, bool) {
	var s saludRespuesta
	resp, err := http.Get(url)
	if err != nil {
		return s, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return s, false
	}
	if json.NewDecoder(resp.Body).Decode(&s) != nil {
		return saludRespuesta{}, false
	}
	return s, true
}

// esperarSalud bloquea hasta que url responde 200 OK DE ESTE PROCESO (su
// pid) o pasa algo malo: el servidor murió (errServidor trae su error) o
// se agota el tiempo total. intervalo es cada cuánto preguntar; total,
// cuánto esperar como máximo.
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
		// Un 200 con otro pid (o sin pid) es de una instancia vieja que
		// todavía tenía el puerto: no es nuestro servidor, seguir esperando.
		if s, ok := leerSalud(url); ok && s.Pid == os.Getpid() {
			return nil
		}
		if !time.Now().Before(limite) {
			return fmt.Errorf("sin respuesta de %s tras %s", url, total)
		}
		time.Sleep(intervalo)
	}
}

// resultadoSonda es lo que encontró el segundo lanzamiento al preguntarle
// a la instancia que ya estaba abierta.
type resultadoSonda int

const (
	instanciaSinRespuesta resultadoSonda = iota
	instanciaMisma
	instanciaOtraVersion
)

// sondearInstancia pregunta a url hasta que alguien responde con su
// versión o se agota total. Compara contra la versión de ESTE binario.
func sondearInstancia(url, version string, intervalo, total time.Duration) resultadoSonda {
	limite := time.Now().Add(total)
	for {
		if s, ok := leerSalud(url); ok && s.Version != "" {
			if s.Version == version {
				return instanciaMisma
			}
			return instanciaOtraVersion
		}
		if !time.Now().Before(limite) {
			return instanciaSinRespuesta
		}
		time.Sleep(intervalo)
	}
}

// segundoLanzamiento se corre cuando ya hay otro Bythos abierto: en vez
// de levantar un segundo servidor, trae al frente la ventana existente
// (misma versión) o avisa por qué no puede. Devuelve el código de salida.
// abrir y avisar vienen de afuera para poder probarlo sin ventanas reales.
func segundoLanzamiento(urlSalud, urlApp, version string, intervalo, total time.Duration,
	abrir func(string) error, avisar func(titulo, mensaje string)) int {
	switch sondearInstancia(urlSalud, version, intervalo, total) {
	case instanciaMisma:
		if err := abrir(urlApp); err != nil {
			// abrir ya mostró su propio aviso nativo (falta Edge).
			return 1
		}
		return 0
	case instanciaOtraVersion:
		avisar("Bythos", msgOtraVersion)
	default:
		avisar("Bythos", msgSinRespuesta)
	}
	return 1
}
