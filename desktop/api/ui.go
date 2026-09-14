package api

// ui.go — El TRUCO del descargable: un solo .exe trae UI + API.
// Sin esto tendrías 2 programas (Go en :8080 y Vite en :5173).
// Con esto, http://localhost:8080/ muestra tu React y
// http://localhost:8080/api/... responde JSON. Una sola puerta.
//
// Dos modos, en orden:
// 1. EMBEBIDO (el .exe final): main.go incrusta ui/dist con go:embed
//    y lo inyecta en Servidor.UI. Viaja DENTRO del binario.
// 2. DISCO (dev): si UI es nil, lee ./ui/dist del disco.
// Ayer solo existía el 2; hoy agregamos el 1 sin romper el 2.

import (
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
)

// UI, si no es nil, son los archivos de ui/dist DENTRO del .exe.
// Lo pone main.go (el único que puede usar go:embed con la ruta correcta).
// ¿Por qué fs.FS (interfaz) y no embed.FS concreto?
// Porque así montarUI acepta lo embebido, un zip o un mock de test.
// Depender de interfaces, no de concretos: senior.
func (s *Servidor) montarUI(mux *http.ServeMux) {
	// Modo 1: embebido (produce el descargable de 1 archivo)
	if s.UI != nil {
	// dist/ tiene index.html en la raíz del FS embebido
	// Si solo hay .gitkeep (aún no hiciste pnpm build), el FileServer
	// mostraría un listado crudo de archivos: confunde. Mismo aviso que disco.
	if _, err := fs.Stat(s.UI, "index.html"); err != nil {
		mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.Write([]byte("Bythos API viva. Falta la UI: entra a desktop/ui y corre `pnpm install` + `pnpm build`.\n"))
		})
		return
	}
	mux.Handle("GET /", http.FileServer(http.FS(s.UI)))
	return
}

	// Modo 2: disco (dev, como ayer)
	dist := filepath.Join("ui", "dist")

	// Si aún no hiciste `pnpm build`, no hay dist: avisa en español
	// en vez de dar un 404 mudo que te hace dudar de Go.
	if _, err := os.Stat(filepath.Join(dist, "index.html")); err != nil {
		mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.Write([]byte("Bythos API viva. Falta la UI: entra a desktop/ui y corre `pnpm install` + `pnpm build`.\n"))
		})
		return
	}

	// FileServer sirve index.html, assets/*.js, *.css...
	archivos := http.FileServer(http.Dir(dist))
	mux.Handle("GET /", archivos)
}
