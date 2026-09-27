package agentes

// escritura_test.go — round trip de escritura extremo a extremo:
// crear_carpeta -> guardar_link -> actualizar_progreso, más las
// validaciones (progreso fuera de rango, estado inválido).

import "testing"

func TestCrearCarpetaGuardarLinkActualizarProgresoRoundTrip(t *testing.T) {
	c, _ := bythosDePrueba(t)
	sesion := sesionDePrueba(t, c)

	var carpeta CarpetaSalida
	isErr, msg := llamar(t, sesion, "crear_carpeta", map[string]any{"nombre": "Go Backend"}, &carpeta)
	if isErr {
		t.Fatalf("crear_carpeta devolvió error: %s", msg)
	}
	if carpeta.ID == 0 || carpeta.Nombre != "Go Backend" {
		t.Fatalf("carpeta creada inesperada: %+v", carpeta)
	}

	var recurso RecursoSalida
	isErr, msg = llamar(t, sesion, "guardar_link", map[string]any{
		"carpeta_id": carpeta.ID,
		"url":        "http://ejemplo.com/handlers",
	}, &recurso)
	if isErr {
		t.Fatalf("guardar_link devolvió error: %s", msg)
	}
	if recurso.ID == 0 || recurso.CarpetaID != carpeta.ID || recurso.Estado != "pendiente" || recurso.Progreso != 0 {
		t.Fatalf("recurso guardado inesperado: %+v", recurso)
	}

	var ok salidaOK
	isErr, msg = llamar(t, sesion, "actualizar_progreso", map[string]any{
		"recurso_id": recurso.ID,
		"progreso":   40,
	}, &ok)
	if isErr {
		t.Fatalf("actualizar_progreso devolvió error: %s", msg)
	}
	if !ok.OK {
		t.Fatalf("esperaba ok:true, salió %+v", ok)
	}

	// El round trip cierra leyendo de nuevo: el progreso y el estado
	// (en_curso, por estar entre 1 y 99) tienen que reflejar el cambio.
	var detalle RecursoDetalle
	isErr, msg = llamar(t, sesion, "leer_recurso", map[string]any{"id": recurso.ID}, &detalle)
	if isErr {
		t.Fatalf("leer_recurso devolvió error: %s", msg)
	}
	if detalle.Progreso != 40 || detalle.Estado != "en_curso" {
		t.Fatalf("progreso no se sincronizó: %+v", detalle)
	}
}

func TestActualizarProgresoFueraDeRango(t *testing.T) {
	c, base := bythosDePrueba(t)
	carpeta := crearCarpetaSemilla(t, base, "Temp")
	recurso := guardarRecursoSemilla(t, base, carpeta, "http://ejemplo.com/x")

	sesion := sesionDePrueba(t, c)

	for _, progreso := range []int{-1, 101, 1000} {
		isErr, msg := llamar(t, sesion, "actualizar_progreso", map[string]any{
			"recurso_id": recurso,
			"progreso":   progreso,
		}, nil)
		if !isErr {
			t.Fatalf("progreso=%d debía rechazarse como tool error", progreso)
		}
		if msg == "" {
			t.Fatalf("progreso=%d: el error debía traer mensaje", progreso)
		}
	}
}

func TestCambiarEstadoInvalido(t *testing.T) {
	c, base := bythosDePrueba(t)
	carpeta := crearCarpetaSemilla(t, base, "Temp")
	recurso := guardarRecursoSemilla(t, base, carpeta, "http://ejemplo.com/y")

	sesion := sesionDePrueba(t, c)

	isErr, msg := llamar(t, sesion, "cambiar_estado", map[string]any{
		"recurso_id": recurso,
		"estado":     "viendo",
	}, nil)
	if !isErr {
		t.Fatalf("estado inválido debía rechazarse como tool error")
	}
	if msg == "" {
		t.Fatalf("el error debía traer mensaje")
	}

	var ok salidaOK
	isErr, msg = llamar(t, sesion, "cambiar_estado", map[string]any{
		"recurso_id": recurso,
		"estado":     "completado",
	}, &ok)
	if isErr {
		t.Fatalf("cambiar_estado (completado) devolvió error: %s", msg)
	}
	if !ok.OK {
		t.Fatalf("esperaba ok:true, salió %+v", ok)
	}
}

func TestCrearNotaAgenda(t *testing.T) {
	c, _ := bythosDePrueba(t)
	sesion := sesionDePrueba(t, c)

	var nota AgendaSalida
	isErr, msg := llamar(t, sesion, "crear_nota_agenda", map[string]any{
		"fecha": "2026-02-01",
		"texto": "repasar closures",
	}, &nota)
	if isErr {
		t.Fatalf("crear_nota_agenda devolvió error: %s", msg)
	}
	if nota.ID == 0 || nota.Fecha != "2026-02-01" || nota.Texto != "repasar closures" {
		t.Fatalf("nota creada inesperada: %+v", nota)
	}
}
