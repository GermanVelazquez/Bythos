package api

// puente_lan_test.go — El adaptador de lan.Biblioteca: guarda con la misma
// secuencia que la UI y deja el evento con origen "celular" y el nombre del
// dispositivo como actor; tipo rechazado y carpeta inexistente no dejan
// nada ni registran evento.

import (
	"bytes"
	"errors"
	"testing"

	"bythos-desktop/archivos"
	"bythos-desktop/db"
	"bythos-desktop/lan"
)

func puenteDePrueba(t *testing.T) (*puenteLAN, int64) {
	t.Helper()
	archivos.UsarBase(t.TempDir())
	t.Cleanup(func() { archivos.UsarBase("") })
	s := basePrueba(t)
	c, _ := db.CrearCarpeta(s.Base, "Go")
	return &puenteLAN{s: s, metadata: func(string) Metadata { return Metadata{Titulo: "Título remoto", Tipo: "articulo"} }}, c.ID
}

func TestPuenteLANArchivoPermitidoDedupeYEvento(t *testing.T) {
	p, carpeta := puenteDePrueba(t)
	png := []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a, 0, 0, 0, 0}

	id1, err := p.GuardarArchivo(carpeta, bytes.NewReader(png), "foto.png", "Pixel de Ana")
	if err != nil || id1 == 0 {
		t.Fatalf("GuardarArchivo: %d %v", id1, err)
	}
	if _, err := p.GuardarArchivo(carpeta, bytes.NewReader(png), "otra.png", "Pixel de Ana"); err != nil {
		t.Fatalf("segundo guardado: %v", err)
	}
	var n int
	if err := p.s.Base.QueryRow(`SELECT COUNT(*) FROM archivos`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("dedupe por hash: %d filas en archivos (err=%v), quería 1", n, err)
	}
	eventos, _ := db.ListarEventos(p.s.Base, db.FiltroEventos{})
	if len(eventos) != 2 || eventos[0].Origen != db.OrigenCelular || eventos[0].Actor != "Pixel de Ana" || eventos[0].Accion != db.AccionArchivoSubido {
		t.Fatalf("eventos mal: %+v", eventos)
	}
}

func TestPuenteLANRechazaTipoYCarpetaInexistente(t *testing.T) {
	p, carpeta := puenteDePrueba(t)

	_, err := p.GuardarArchivo(carpeta, bytes.NewReader([]byte("<html><script>alert(1)</script></html>")), "x.png", "Pixel")
	if !errors.Is(err, archivos.ErrTipoNoPermitido) {
		t.Fatalf("HTML disfrazado: err=%v, quería ErrTipoNoPermitido", err)
	}
	if _, err := p.GuardarArchivo(999, bytes.NewReader([]byte("x")), "x.txt", "Pixel"); !errors.Is(err, lan.ErrCarpetaNoExiste) {
		t.Fatalf("carpeta inexistente: err=%v", err)
	}
	if _, _, err := p.GuardarLink(999, "https://example.com", "", "Pixel"); !errors.Is(err, lan.ErrCarpetaNoExiste) {
		t.Fatalf("link a carpeta inexistente: err=%v", err)
	}
	if eventos, _ := db.ListarEventos(p.s.Base, db.FiltroEventos{}); len(eventos) != 0 {
		t.Fatalf("los rechazos no deben registrar eventos: %+v", eventos)
	}
}

func TestPuenteLANLinkUsaMetadataYRegistraOrigen(t *testing.T) {
	p, carpeta := puenteDePrueba(t)
	id, titulo, err := p.GuardarLink(carpeta, "https://example.com/a", "", "Pixel de Ana")
	if err != nil || id == 0 || titulo != "Título remoto" {
		t.Fatalf("GuardarLink: %d %q %v", id, titulo, err)
	}
	_, titulo, _ = p.GuardarLink(carpeta, "https://example.com/b", "Mío", "Pixel de Ana")
	if titulo != "Mío" {
		t.Fatalf("el título manual debía respetarse, salió %q", titulo)
	}
	eventos, _ := db.ListarEventos(p.s.Base, db.FiltroEventos{})
	if len(eventos) != 2 || eventos[0].Origen != db.OrigenCelular || eventos[0].Accion != db.AccionRecursoGuardado {
		t.Fatalf("eventos mal: %+v", eventos)
	}
	if carpetas, err := p.ListarCarpetas(); err != nil || len(carpetas) != 1 || carpetas[0].Nombre != "Go" {
		t.Fatalf("ListarCarpetas: %+v %v", carpetas, err)
	}
}
