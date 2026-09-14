package ventana

// ventana.go — La VENTANA de verdad. Doble clic → ventana Bythos, sin Chrome a la vista.
// ¿Cómo? Abre tu Edge en modo --app: ventana sin pestañas ni barra de direcciones,
// con tu React adentro. Parece nativa porque usa el motor del sistema (WebView2).
//
// ¿Por qué NO Wails ni webview CGO? Te lo digo honesto: piden gcc en Windows,
// 200MB de toolchain y Node. Verifiqué tu PC: no hay gcc. Esta vía usa el Edge
// que YA tienes, cero dependencias nuevas, y tu Go sigue compilando puro.
// Cuando el proyecto pida menú nativo o bandeja, migramos a Wails con motivo.

import (
	"os"
	"os/exec"
)

// Abrir lanza la ventana y vuelve al instante (no bloquea al servidor).
// Si no hay Edge/Chrome, devuelve error y main sigue: igual puedes usar
// el navegador a mano en localhost. Degradar, nunca morir.
func Abrir(url string) error {
	// Orden: Edge (viene con Windows) → Chrome → plan B del sistema.
	candidatos := []string{
		`C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`,
		`C:\Program Files\Microsoft\Edge\Application\msedge.exe`,
		os.ExpandEnv(`$LOCALAPPDATA\Google\Chrome\Application\chrome.exe`),
		`C:\Program Files\Google\Chrome\Application\chrome.exe`,
	}
	for _, exe := range candidatos {
		if _, err := os.Stat(exe); err != nil {
			continue // no está instalado, probar el siguiente
		}
		// --app=url = ventana sola, sin pestañas. Start (no Run): no bloquea.
		cmd := exec.Command(exe, "--app="+url, "--user-data-dir="+perfilBythos())
		if err := cmd.Start(); err == nil {
			return nil
		}
	}
	// Plan B: abre el navegador que sea (mejor eso que nada).
	return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
}

// perfilBythos guarda cookies/estado de la ventana aparte de tu Edge normal.
// Así tu Bythos no mezcla sesiones con tu navegación personal.
func perfilBythos() string {
	base, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return base + `\Bythos\Ventana`
}
