package workspace_test

// Las Instructions del servidor MCP llevan las dos reglas (biblioteca sin
// borrar + pizarra). Paquete externo: agentes ya importa workspace.

import (
	"strings"
	"testing"

	"bythos-desktop/agentes"
	"bythos-desktop/workspace"
)

func TestInstruccionesMCPTraenAmbasReglas(t *testing.T) {
	for _, frase := range []string{workspace.ReglaBiblioteca, workspace.ReglaPizarra} {
		if !strings.Contains(agentes.Instrucciones, frase) {
			t.Fatalf("Instructions sin la frase %q", frase)
		}
	}
}
