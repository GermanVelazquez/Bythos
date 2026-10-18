package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
)

// Las fuentes de la pizarra (.woff2) se sirven como font/woff2, no como
// text/plain ni por lo que diga el registro de Windows.
func TestUIServeWoff2ConSuTipo(t *testing.T) {
	s := &Servidor{UI: fstest.MapFS{
		"index.html":                      {Data: []byte("<html></html>")},
		"excalidraw/fonts/Nunito/a.woff2": {Data: []byte("wOF2")},
	}}
	mux := http.NewServeMux()
	s.montarUI(mux)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest("GET", "/excalidraw/fonts/Nunito/a.woff2", nil))
	if ct := rr.Header().Get("Content-Type"); ct != "font/woff2" {
		t.Fatalf("Content-Type = %q, quiero font/woff2", ct)
	}
}
