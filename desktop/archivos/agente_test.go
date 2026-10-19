package archivos

// agente_test.go — Lo que el agente lee de un adjunto: imagen acotada a
// 1280px y texto plano con tope de 40.000 caracteres (design D13, D14).

import (
	"bytes"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func escribirBlob(t *testing.T, rel string, datos []byte) {
	t.Helper()
	UsarBase(t.TempDir())
	t.Cleanup(func() { UsarBase("") })
	destino := Absoluta(rel)
	if err := os.MkdirAll(filepath.Dir(destino), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(destino, datos, 0644); err != nil {
		t.Fatal(err)
	}
}

func pngDe(t *testing.T, ancho, alto int) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, ancho, alto))); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestImagenParaAgenteEscalaAl1280(t *testing.T) {
	escribirBlob(t, "ab/grande.png", pngDe(t, 1440, 720))
	img, err := ImagenParaAgente("ab/grande.png", "image/png")
	if err != nil || len(img.Datos) == 0 || img.Mime != "image/png" {
		t.Fatalf("imagen: %v %+v", err, img)
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(img.Datos))
	if err != nil || cfg.Width != 1280 || cfg.Height != 640 {
		t.Fatalf("dimensiones %dx%d (%v), quería 1280x640", cfg.Width, cfg.Height, err)
	}
}

func TestImagenParaAgenteChicaSeDevuelveTalCual(t *testing.T) {
	original := pngDe(t, 100, 50)
	escribirBlob(t, "ab/chica.png", original)
	img, err := ImagenParaAgente("ab/chica.png", "image/png")
	if err != nil || !bytes.Equal(img.Datos, original) {
		t.Fatalf("una imagen chica no se re-codifica: %v", err)
	}
}

func TestImagenParaAgenteWebpYNoDecodificable(t *testing.T) {
	webp := []byte("RIFF\x00\x00\x00\x00WEBPVP8 ")
	escribirBlob(t, "ab/x.webp", webp)
	img, err := ImagenParaAgente("ab/x.webp", "image/webp")
	if err != nil || !bytes.Equal(img.Datos, webp) || img.Mime != "image/webp" {
		t.Fatalf("webp chico va tal cual: %v %+v", err, img)
	}

	roto := []byte("no soy un png")
	escribirBlob(t, "ab/roto.png", roto)
	if img, err := ImagenParaAgente("ab/roto.png", "image/png"); err != nil || !bytes.Equal(img.Datos, roto) {
		t.Fatalf("no decodificable chico va tal cual: %v", err)
	}

	grande := bytes.Repeat([]byte{1}, maxBytesSinProcesar+1)
	escribirBlob(t, "ab/enorme.webp", grande)
	img, err = ImagenParaAgente("ab/enorme.webp", "image/webp")
	if err != nil || img.Datos != nil || img.Aviso == "" {
		t.Fatalf("webp de más de 5 MiB: solo aviso: %v %+v", err, img.Aviso)
	}
}

func TestImagenParaAgenteArchivoInexistente(t *testing.T) {
	UsarBase(t.TempDir())
	t.Cleanup(func() { UsarBase("") })
	if _, err := ImagenParaAgente("ab/nada.png", "image/png"); err == nil {
		t.Fatal("un archivo inexistente debe fallar")
	}
}

func TestPlanoAplanaCadaFormato(t *testing.T) {
	casos := []struct {
		nombre string
		t      TextoExtraido
		quiere string
	}{
		{"docx", TextoExtraido{Tipo: "docx", Bloques: []BloqueTexto{
			{Tipo: "titulo", Nivel: 1, Texto: "Plan"}, {Tipo: "parrafo", Texto: "Hola"}, {Tipo: "lista", Texto: "uno"}}},
			"# Plan\nHola\n- uno"},
		{"pptx", TextoExtraido{Tipo: "pptx", Diapositivas: []Diapositiva{
			{Numero: 1, Parrafos: []string{"A", "B"}}, {Numero: 2, Parrafos: []string{"C"}}}},
			"Diapositiva 1\nA\nB\nDiapositiva 2\nC"},
		{"xlsx", TextoExtraido{Tipo: "xlsx", Filas: [][]string{{"a", "b"}, {"c", "d"}}}, "a\tb\nc\td"},
		{"texto", TextoExtraido{Tipo: "texto", Texto: "libre"}, "libre"},
	}
	for _, c := range casos {
		p := c.t.Plano()
		if p.Texto != c.quiere || p.Truncado || p.CaracteresTotales != len([]rune(c.quiere)) {
			t.Errorf("%s: %+v, quería %q", c.nombre, p, c.quiere)
		}
	}
}

func TestPlanoTruncaA40000Runas(t *testing.T) {
	largo := strings.Repeat("ñ", maxRunasAgente+500)
	p := TextoExtraido{Tipo: "texto", Texto: largo}.Plano()
	if !p.Truncado || len([]rune(p.Texto)) != maxRunasAgente || p.CaracteresTotales != maxRunasAgente+500 {
		t.Fatalf("truncado=%v runas=%d totales=%d", p.Truncado, len([]rune(p.Texto)), p.CaracteresTotales)
	}
	if p := (TextoExtraido{Tipo: "texto", Texto: "corto", Truncado: true}).Plano(); !p.Truncado {
		t.Fatal("el truncado del extractor se conserva")
	}
}
