package agentes

// pizarra_test.go — Las tres tools de la pizarra contra un httptest que
// imita api/pizarra.go (el contrato HTTP ya está probado allá).

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var pngMinimo = []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a, 1, 2, 3}

// pizarraFalsa devuelve un cliente contra un servidor que responde según la ruta.
func pizarraFalsa(t *testing.T, h http.HandlerFunc) (*Cliente, *atomic.Int32) {
	t.Helper()
	var llamadas atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		llamadas.Add(1)
		h(w, r)
	}))
	t.Cleanup(srv.Close)
	return &Cliente{BaseURL: srv.URL, Host: "localhost:8080", HTTP: srv.Client(), DirAgente: t.TempDir()}, &llamadas
}

func TestPizarraLeerForma(t *testing.T) {
	t.Setenv("BYTHOS_SESION", "7")
	var consulta string
	c, _ := pizarraFalsa(t, func(w http.ResponseWriter, r *http.Request) {
		consulta = r.URL.Path + "?" + r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"sesion":{"id":7},"resolucion":"entorno","version":3,"actualizado":"x","elementos":[],"resumen":{"total":0,"por_tipo":{}},"limites":{},"ops_pendientes":0,"truncado":false}`)
	})
	var salida map[string]any
	if err, txt := llamar(t, sesionDePrueba(t, c), "pizarra_leer", map[string]any{"sesion_id": 7}, &salida); err {
		t.Fatalf("error inesperado: %s", txt)
	}
	for _, k := range []string{"sesion", "resolucion", "version", "actualizado", "elementos", "resumen", "limites", "ops_pendientes", "truncado"} {
		if _, ok := salida[k]; !ok {
			t.Errorf("falta %q en %v", k, salida)
		}
	}
	if !strings.Contains(consulta, "/api/terminal/pizarra") || !strings.Contains(consulta, "sesion_id=7") || !strings.Contains(consulta, "pista=7") {
		t.Errorf("consulta inesperada: %s", consulta)
	}
}

func TestPizarraDibujarValidaTemprano(t *testing.T) {
	c, llamadas := pizarraFalsa(t, func(w http.ResponseWriter, r *http.Request) {})
	sesion := sesionDePrueba(t, c)
	isErr, txt := llamar(t, sesion, "pizarra_dibujar", map[string]any{"ops": []any{map[string]any{"accion": "borrar"}}}, nil)
	if !isErr || txt == "" {
		t.Fatalf("esperaba error de validación, isError=%v txt=%q", isErr, txt)
	}
	if llamadas.Load() != 0 {
		t.Errorf("una entrada inválida no debe llegar a HTTP (llamadas=%d)", llamadas.Load())
	}
}

func TestPizarraDibujarEnviaOps(t *testing.T) {
	t.Setenv("BYTHOS_SESION", "4")
	var cuerpo map[string]any
	c, _ := pizarraFalsa(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/terminal/pizarra/ops" || r.Method != "POST" {
			t.Errorf("ruta inesperada %s %s", r.Method, r.URL.Path)
		}
		json.NewDecoder(r.Body).Decode(&cuerpo)
		io.WriteString(w, `{"op_id":1,"sesion_id":4,"resolucion":"entorno","estado":"aplicada","ids":["a"]}`)
	})
	ops := []any{map[string]any{"accion": "agregar", "elemento": map[string]any{"id": "a", "tipo": "rectangle", "x": 0, "y": 0}}}
	var salida map[string]any
	if isErr, txt := llamar(t, sesionDePrueba(t, c), "pizarra_dibujar", map[string]any{"ops": ops}, &salida); isErr {
		t.Fatalf("error inesperado: %s", txt)
	}
	if salida["estado"] != "aplicada" {
		t.Errorf("salida = %v", salida)
	}
	if cuerpo["pista"] != "4" || len(cuerpo["ops"].([]any)) != 1 {
		t.Errorf("cuerpo = %v", cuerpo)
	}
	if _, hay := cuerpo["sesion_id"]; hay {
		t.Errorf("sin sesion_id no debe mandarse: %v", cuerpo)
	}
}

func TestPizarraCapturaImagenTextoYArchivo(t *testing.T) {
	c, _ := pizarraFalsa(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("X-Bythos-Sesion", "9")
		w.Header().Set("X-Bythos-Captura", "5")
		w.Header().Set("X-Bythos-Resolucion", "activa")
		w.Write(pngMinimo)
	})
	res, err := sesionDePrueba(t, c).CallTool(context.Background(), &mcp.CallToolParams{Name: "pizarra_captura"})
	if err != nil || res.IsError {
		t.Fatalf("err=%v res=%+v", err, res)
	}
	if res.StructuredContent != nil {
		t.Errorf("StructuredContent debe ser nil: %v", res.StructuredContent)
	}
	if len(res.Content) != 2 {
		t.Fatalf("esperaba 2 contenidos, llegaron %d", len(res.Content))
	}
	img, ok := res.Content[0].(*mcp.ImageContent)
	if !ok || img.MIMEType != "image/png" || string(img.Data) != string(pngMinimo) {
		t.Errorf("imagen inesperada: %#v", res.Content[0])
	}
	txt, ok := res.Content[1].(*mcp.TextContent)
	if !ok {
		t.Fatalf("segundo contenido no es texto: %#v", res.Content[1])
	}
	ruta := filepath.Join(c.DirAgente, "capturas", "pizarra-9.png")
	if !strings.Contains(txt.Text, ruta) {
		t.Errorf("el texto no cita %s: %s", ruta, txt.Text)
	}
	if datos, err := os.ReadFile(ruta); err != nil || string(datos) != string(pngMinimo) {
		t.Errorf("archivo mal escrito: %v", err)
	}
}

func TestPizarraCapturaError409(t *testing.T) {
	c, _ := pizarraFalsa(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(409)
		io.WriteString(w, `{"error":"La pantalla Terminal no está abierta"}`)
	})
	isErr, txt := llamar(t, sesionDePrueba(t, c), "pizarra_captura", nil, nil)
	if !isErr || !strings.Contains(txt, "Terminal no está abierta") {
		t.Errorf("isError=%v txt=%q", isErr, txt)
	}
}

// La captura usa el cliente largo: con HTTP a 50 ms una respuesta de 200 ms
// solo la sobrevive pizarra_captura.
func TestPizarraCapturaUsaClienteLargo(t *testing.T) {
	c, _ := pizarraFalsa(t, func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.Header().Set("X-Bythos-Sesion", "1")
		w.Write(pngMinimo)
	})
	c.HTTP = &http.Client{Timeout: 50 * time.Millisecond}
	sesion := sesionDePrueba(t, c)
	if isErr, txt := llamar(t, sesion, "pizarra_captura", nil, nil); isErr {
		t.Fatalf("la captura debía sobrevivir al cliente corto: %s", txt)
	}
	if isErr, _ := llamar(t, sesion, "pizarra_leer", nil, nil); !isErr {
		t.Error("pizarra_leer debía vencer con el cliente de 15 s (aquí 50 ms)")
	}
	if TimeoutLargo != 25*time.Second {
		t.Errorf("TimeoutLargo = %v", TimeoutLargo)
	}
}

// La API decodifica sesion_id como entero: un string daría 400.
func TestPizarraCapturaMandaSesionIDNumerico(t *testing.T) {
	c, _ := pizarraFalsa(t, func(w http.ResponseWriter, r *http.Request) {
		var b struct {
			SesionID *int64 `json:"sesion_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&b); err != nil || b.SesionID == nil || *b.SesionID != 9 {
			http.Error(w, `{"error":"JSON inválido"}`, 400)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("X-Bythos-Sesion", "9")
		w.Write(pngMinimo)
	})
	if isErr, txt := llamar(t, sesionDePrueba(t, c), "pizarra_captura", map[string]any{"sesion_id": 9}, nil); isErr {
		t.Fatalf("sesion_id numérico rechazado: %s", txt)
	}
}
