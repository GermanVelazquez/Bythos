package agentes

// historial_test.go — el historial visto desde MCP: una escritura por una
// tool debe quedar con origen "agente" y el nombre del cliente MCP como
// actor (ver conActor/actorDeClientInfo), y ver_historial debe poder leerla.

import (
	"testing"

	"bythos-desktop/db"
)

func TestEscrituraRegistraOrigenAgenteYActorDelClienteMCP(t *testing.T) {
	c, base := bythosDePrueba(t)
	sesion := sesionDePrueba(t, c) // sesionDePrueba conecta con Implementation.Name "test-agent"

	var carpeta CarpetaSalida
	isErr, msg := llamar(t, sesion, "crear_carpeta", map[string]any{"nombre": "Go Backend"}, &carpeta)
	if isErr {
		t.Fatalf("crear_carpeta devolvió error: %s", msg)
	}

	eventos, err := db.ListarEventos(base, db.FiltroEventos{})
	if err != nil || len(eventos) != 1 {
		t.Fatalf("listar eventos mal: %+v err=%v", eventos, err)
	}
	e := eventos[0]
	if e.Origen != db.OrigenAgente {
		t.Fatalf("origen mal: %q", e.Origen)
	}
	if e.Actor != "test-agent" {
		t.Fatalf("actor mal: %q, quería el nombre del cliente MCP de sesionDePrueba", e.Actor)
	}
	if e.Accion != db.AccionCarpetaCreada {
		t.Fatalf("accion mal: %q", e.Accion)
	}
}

func TestVerHistorialDevuelveElEventoRegistrado(t *testing.T) {
	c, _ := bythosDePrueba(t)
	sesion := sesionDePrueba(t, c)

	var carpeta CarpetaSalida
	isErr, msg := llamar(t, sesion, "crear_carpeta", map[string]any{"nombre": "Go Backend"}, &carpeta)
	if isErr {
		t.Fatalf("crear_carpeta devolvió error: %s", msg)
	}

	var salida salidaVerHistorial
	isErr, msg = llamar(t, sesion, "ver_historial", nil, &salida)
	if isErr {
		t.Fatalf("ver_historial devolvió error: %s", msg)
	}
	if len(salida.Eventos) != 1 {
		t.Fatalf("quería 1 evento, salieron %d: %+v", len(salida.Eventos), salida.Eventos)
	}
	ev := salida.Eventos[0]
	if ev.Origen != "agente" || ev.Actor != "test-agent" || ev.Accion != "carpeta_creada" {
		t.Fatalf("evento mal: %+v", ev)
	}

	// origen inválido se rechaza como tool error, no llega a pegarle a la API.
	isErr, msg = llamar(t, sesion, "ver_historial", map[string]any{"origen": "quien-sabe"}, nil)
	if !isErr {
		t.Fatalf("origen inválido debía rechazarse como tool error")
	}
	if msg == "" {
		t.Fatalf("el error debía traer mensaje")
	}
}
