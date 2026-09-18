package main

// main.go — El DIRECTOR. Solo orquesta, no piensa.
// Si este archivo pasa de 60 líneas, algo estás metiendo mal.
// Conecta 4 piezas: memoria (.db) + cara embebida (dist) + cerebro (:8080) + ventana.

import (
	"embed"
	"io/fs"
	"log"
	"net/http"
	"os"
	"time"

	"bythos-desktop/api"
	"bythos-desktop/db"
	"bythos-desktop/ventana"
)

// El .exe lleva la UI ADENTRO: al compilar, Go copia ui/dist al binario.
// ¿Por qué aquí y no en api/? Porque go:embed solo acepta rutas relativas
// al archivo que lo declara, y "ui/dist" solo es correcto desde desktop/.
// Regla: el embed vive donde la ruta tiene sentido (el dueño del layout).
//
//go:embed all:ui/dist
var distUI embed.FS

func main() {
	// 1. Memoria: abre (o crea) %APPDATA%/Bythos/bythos.db
	// ¿Por qué aquí y no dentro de api? Porque el dueño del recurso
	// es quien lo cierra. Main abre, main cierra con defer.
	ruta := db.RutaPorDefecto()
	base, err := db.Abrir(ruta)
	if err != nil {
		fallarArranque("No se pudo abrir bythos.db: " + err.Error())
	}
	defer base.Close()

	// 2. Cara: recorta "ui/dist" del FS embebido.
	// embed guarda rutas completas ("ui/dist/index.html"); fs.Sub deja
	// la raíz en dist/ para que FileServer sirva "/" directo.
	// Si aún no hiciste pnpm build, solo hay .gitkeep y montarUI avisará.
	sub, err := fs.Sub(distUI, "ui/dist")
	if err != nil {
		base.Close()
		fallarArranque("No se pudo leer la UI embebida: " + err.Error())
	}

	// 3. Cerebro: sirve la API para la UI y la extensión (+ la cara en "/")
	srv := &api.Servidor{Base: base, UI: sub}

	// 4. Ventana: el servidor va en segundo plano y la ventana al frente.
	// ¿Por qué goroutine? ListenAndServe BLOQUEA (nunca vuelve); sin go,
	// la ventana jamás abriría. La ventana es la que manda a cerrar: si el
	// usuario la cierra, el .exe sigue hasta cerrar la consola (v1 simple).
	go func() {
		log.Println("Bythos listo.")
		log.Println("BD en:", ruta)
		log.Println("API en: http://localhost:8080/api/salud")
		if err := http.ListenAndServe("localhost:8080", srv.Rutas()); err != nil {
			fallarArranque("No se pudo prender el servidor local: " + err.Error())
		}
	}()

	// Esperar a que el puerto despierte antes de abrir la ventana:
	// sin esto Edge llegaría a una puerta cerrada 1 de cada 3 veces.
	time.Sleep(800 * time.Millisecond)
	if err := ventana.Abrir("http://localhost:8080"); err != nil {
		// Abrir ya mostró su propio aviso nativo (falta Edge):
		// no duplicar diálogos, solo dejar rastro y salir.
		log.Println("No se pudo abrir la ventana, abre el navegador en http://localhost:8080:", err)
		os.Exit(1)
	}

	// El director no se va: bloquea para siempre (la ventana y el server viven).
	select {}
}

// fallarArranque deja rastro en la terminal (dev con `go run .`) Y muestra
// un diálogo nativo (instalado sin consola con -H=windowsgui, donde el log
// no se ve en ningún lado). Siempre termina con código 1.
func fallarArranque(detalle string) {
	log.Println(detalle)
	ventana.Error("Bythos", detalle)
	os.Exit(1)
}
