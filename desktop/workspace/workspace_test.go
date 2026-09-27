package workspace

// workspace_test.go — PrepararWorkspace escribe JSON/TOML correctos, con
// el .exe embebido a salvo de barras de Windows, es idempotente y nunca
// borra un archivo que no conoce. Usa una carpeta temporal propia (no
// RutaWorkspace real) inyectando HOME/APPDATA con t.Setenv, así nunca
// toca el %APPDATA% de quien corre `go test`.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// conWorkspaceTemporal apunta os.UserConfigDir() a un t.TempDir() (vía
// %APPDATA% en Windows, $HOME/Library/... en otros SO) y devuelve la
// carpeta desktop/agente que PrepararWorkspace va a usar.
func conWorkspaceTemporal(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("APPDATA", tmp)         // os.UserConfigDir() en Windows
	t.Setenv("XDG_CONFIG_HOME", tmp) // por si el test corre en Linux/mac
	t.Setenv("HOME", tmp)
	return filepath.Join(tmp, "Bythos", "agente")
}

const exeDePrueba = `C:\Users\german\AppData\Local\Bythos\bythos.exe`

func TestPrepararWorkspaceCreaArchivosConJSONValido(t *testing.T) {
	esperado := conWorkspaceTemporal(t)
	dir, err := PrepararWorkspace(exeDePrueba)
	if err != nil {
		t.Fatalf("PrepararWorkspace: %v", err)
	}
	if dir != esperado {
		t.Fatalf("dir = %q, quería %q", dir, esperado)
	}

	for _, nombre := range []string{"AGENTS.md", "CLAUDE.md", "LEEME.txt", ".mcp.json", "opencode.json"} {
		if _, err := os.Stat(filepath.Join(dir, nombre)); err != nil {
			t.Fatalf("falta %s: %v", nombre, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, ".gemini", "settings.json")); err != nil {
		t.Fatalf("falta .gemini/settings.json: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".codex", "config.toml")); err != nil {
		t.Fatalf("falta .codex/config.toml: %v", err)
	}

	// .mcp.json: JSON válido, mcpServers.bythos con el exe y ["mcp"].
	b, err := os.ReadFile(filepath.Join(dir, ".mcp.json"))
	if err != nil {
		t.Fatalf("leer .mcp.json: %v", err)
	}
	var claude struct {
		McpServers map[string]struct {
			Command string   `json:"command"`
			Args    []string `json:"args"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal(b, &claude); err != nil {
		t.Fatalf(".mcp.json no es JSON válido: %v\ncontenido: %s", err, b)
	}
	bythos, ok := claude.McpServers["bythos"]
	if !ok {
		t.Fatalf(".mcp.json sin servidor 'bythos': %s", b)
	}
	if bythos.Command != exeDePrueba {
		t.Fatalf("command = %q, quería %q (barras de Windows deben llegar intactas)", bythos.Command, exeDePrueba)
	}
	if len(bythos.Args) != 1 || bythos.Args[0] != "mcp" {
		t.Fatalf("args = %v, quería [\"mcp\"]", bythos.Args)
	}

	// opencode.json: mcp.bythos con type local y command como ARRAY.
	b, err = os.ReadFile(filepath.Join(dir, "opencode.json"))
	if err != nil {
		t.Fatalf("leer opencode.json: %v", err)
	}
	var opencode struct {
		Mcp map[string]struct {
			Type    string   `json:"type"`
			Command []string `json:"command"`
			Enabled bool     `json:"enabled"`
		} `json:"mcp"`
	}
	if err := json.Unmarshal(b, &opencode); err != nil {
		t.Fatalf("opencode.json no es JSON válido: %v\ncontenido: %s", err, b)
	}
	ocBythos, ok := opencode.Mcp["bythos"]
	if !ok {
		t.Fatalf("opencode.json sin servidor 'bythos': %s", b)
	}
	if ocBythos.Type != "local" {
		t.Fatalf("type = %q, quería local", ocBythos.Type)
	}
	if len(ocBythos.Command) != 2 || ocBythos.Command[0] != exeDePrueba || ocBythos.Command[1] != "mcp" {
		t.Fatalf("command = %v, quería [%q, \"mcp\"]", ocBythos.Command, exeDePrueba)
	}
	if !ocBythos.Enabled {
		t.Fatal("enabled debería ser true")
	}

	// .gemini/settings.json: misma forma que .mcp.json (command+args).
	b, err = os.ReadFile(filepath.Join(dir, ".gemini", "settings.json"))
	if err != nil {
		t.Fatalf("leer .gemini/settings.json: %v", err)
	}
	var gemini struct {
		McpServers map[string]struct {
			Command string   `json:"command"`
			Args    []string `json:"args"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal(b, &gemini); err != nil {
		t.Fatalf(".gemini/settings.json no es JSON válido: %v", err)
	}
	if gemini.McpServers["bythos"].Command != exeDePrueba {
		t.Fatalf("gemini command = %q, quería %q", gemini.McpServers["bythos"].Command, exeDePrueba)
	}

	// .codex/config.toml: el exe viaja como TOML literal string (comillas
	// simples), sin duplicar cada barra a mano.
	b, err = os.ReadFile(filepath.Join(dir, ".codex", "config.toml"))
	if err != nil {
		t.Fatalf("leer config.toml: %v", err)
	}
	toml := string(b)
	if !strings.Contains(toml, "'"+exeDePrueba+"'") {
		t.Fatalf("config.toml no trae el exe como literal string: %s", toml)
	}
	if !strings.Contains(toml, "[mcp_servers.bythos]") {
		t.Fatalf("config.toml sin tabla [mcp_servers.bythos]: %s", toml)
	}
}

func TestPrepararWorkspaceEsIdempotenteYNoBorraArchivosAjenos(t *testing.T) {
	conWorkspaceTemporal(t)
	dir, err := PrepararWorkspace(exeDePrueba)
	if err != nil {
		t.Fatalf("primera pasada: %v", err)
	}

	// El usuario deja su propio archivo en la carpeta.
	propio := filepath.Join(dir, "notas-mias.txt")
	if err := os.WriteFile(propio, []byte("no me toques"), 0644); err != nil {
		t.Fatalf("crear archivo propio: %v", err)
	}

	// Segunda pasada, con un exe DISTINTO (simula que el usuario movió o
	// actualizó el .exe): debe regenerar sin fallar y sin borrar lo propio.
	otroExe := `D:\OtraRuta\bythos.exe`
	dir2, err := PrepararWorkspace(otroExe)
	if err != nil {
		t.Fatalf("segunda pasada: %v", err)
	}
	if dir2 != dir {
		t.Fatalf("la carpeta cambió entre pasadas: %q != %q", dir2, dir)
	}

	if _, err := os.Stat(propio); err != nil {
		t.Fatalf("el archivo propio del usuario no debería desaparecer: %v", err)
	}

	b, err := os.ReadFile(filepath.Join(dir, ".mcp.json"))
	if err != nil {
		t.Fatalf("leer .mcp.json tras regenerar: %v", err)
	}
	var claude struct {
		McpServers map[string]struct {
			Command string `json:"command"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal(b, &claude); err != nil {
		t.Fatalf(".mcp.json no es JSON válido tras regenerar: %v", err)
	}
	if claude.McpServers["bythos"].Command != otroExe {
		t.Fatalf(".mcp.json debería apuntar al exe NUEVO tras regenerar, command=%q quería %q", claude.McpServers["bythos"].Command, otroExe)
	}
}

func TestTomlLiteralEscapaComillaSimplePocoProbable(t *testing.T) {
	// Caso borde: si algún día una ruta trae una comilla simple, no debe
	// romper el TOML (cae a basic string con \ y " escapados).
	raro := `C:\Users\o'brien\bythos.exe`
	lit := tomlLiteral(raro)
	if !strings.HasPrefix(lit, `"`) || !strings.HasSuffix(lit, `"`) {
		t.Fatalf("con comilla simple debería usar basic string, salió: %s", lit)
	}
	if !strings.Contains(lit, `\\`) {
		t.Fatalf("las barras deberían quedar escapadas: %s", lit)
	}
}
