package ventana

// ventana.go — La VENTANA de verdad. Doble clic → ventana Bythos, sin Chrome a la vista.
// ¿Cómo? Abre tu Edge en modo --app: ventana sin pestañas ni barra de direcciones,
// con tu React adentro. Parece nativa porque usa el motor del sistema (WebView2).
//
// ¿Por qué NO Wails ni webview CGO? Te lo digo honesto: piden gcc en Windows,
// 200MB de toolchain y Node. Verifiqué tu PC: no hay gcc. Esta vía usa el Edge
// que YA tienes, cero dependencias nuevas, y tu Go sigue compilando puro.
// Cuando el proyecto pida menú nativo o bandeja, migramos a Wails con motivo.
//
// REGLA DE ORO: la app instalada NUNCA abre el navegador, siempre ventana.
// Si no hay Edge/Chrome, se muestra un aviso NATIVO (MessageBoxW, Go puro,
// sin CGO) y se devuelve error para que main lo loguee. Nada de pestañas.

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
	"unsafe"
)

// Estilos de MessageBoxW (winuser.h): OK solo + icono de error.
const (
	mbAceptar    = 0x00000000
	mbIconoError = 0x00000010
)

// Abrir lanza la ventana y vuelve al instante (no bloquea al servidor).
// Sin Edge/Chrome: aviso nativo + error (main lo loguea). Nunca pestañas.
func Abrir(url string) error {
	return abrir(url, candidatos(), avisarSinNavegador)
}

// abrir es la versión inyectable de Abrir: los tests le pasan candidatos
// falsos y un aviso espiado, así prueban el fallback sin abrir ventanas.
func abrir(url string, exes []string, avisar func()) error {
	// Orden: Edge (viene con Windows) → Chrome → aviso nativo.
	for _, exe := range exes {
		if _, err := os.Stat(exe); err != nil {
			continue // no está instalado, probar el siguiente
		}
		// --app=url = ventana sola, sin pestañas. Start (no Run): no bloquea.
		cmd := exec.Command(exe, "--app="+url, "--user-data-dir="+perfilBythos())
		if err := cmd.Start(); err == nil {
			return nil
		}
	}
	// Sin ventana posible: informar en vez de abrir el navegador.
	// Edge existe en todo Win10/11, así que esto casi nunca salta.
	avisar()
	return errors.New("Bythos necesita Microsoft Edge para abrir su ventana y no se encontró ningún navegador compatible")
}

// candidatos devuelve dónde buscar Edge/Chrome, en orden de preferencia.
func candidatos() []string {
	return []string{
		`C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`,
		`C:\Program Files\Microsoft\Edge\Application\msedge.exe`,
		os.ExpandEnv(`$LOCALAPPDATA\Google\Chrome\Application\chrome.exe`),
		`C:\Program Files\Google\Chrome\Application\chrome.exe`,
	}
}

// textoAviso es el contenido del diálogo, separado para probarlo sin GUI.
func textoAviso() (titulo, mensaje string) {
	return "Bythos",
		"Bythos necesita Microsoft Edge para abrir su ventana.\n" +
			"Edge viene incluido con Windows 10 y 11: restáuralo o instálalo y vuelve a abrir Bythos."
}

// avisarSinNavegador muestra el diálogo nativo de Windows (MessageBoxW).
// Go puro, sin CGO: user32.dll ya está en todo Windows.
func avisarSinNavegador() {
	titulo, mensaje := textoAviso()
	Aviso(titulo, mensaje)
}

// Aviso muestra un diálogo nativo informativo (botón Aceptar).
func Aviso(titulo, mensaje string) {
	t, _ := syscall.UTF16PtrFromString(titulo)
	m, _ := syscall.UTF16PtrFromString(mensaje)
	mostrarMensajeNativo(t, m, mbAceptar)
}

// Error muestra un diálogo nativo de error (icono de error, botón Aceptar).
// Es la vía visible cuando el .exe corre SIN consola (-H=windowsgui):
// ahí log.Fatal no se ve en ningún lado, este diálogo sí.
func Error(titulo, mensaje string) {
	t, _ := syscall.UTF16PtrFromString(titulo)
	m, _ := syscall.UTF16PtrFromString(mensaje)
	mostrarMensajeNativo(t, m, mbAceptar|mbIconoError)
}

// mostrarMensajeNativo llama a MessageBoxW con el estilo pedido.
func mostrarMensajeNativo(titulo, mensaje *uint16, estilo uint32) {
	user32 := syscall.NewLazyDLL("user32.dll")
	mensajeBox := user32.NewProc("MessageBoxW")
	mensajeBox.Call(0, // sin ventana dueña: el diálogo va al frente
		uintptr(unsafe.Pointer(mensaje)),
		uintptr(unsafe.Pointer(titulo)),
		uintptr(estilo))
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
