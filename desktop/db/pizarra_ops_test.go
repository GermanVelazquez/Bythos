package db

import (
	"errors"
	"testing"
)

func TestOpsEncolarYResponder(t *testing.T) {
	base := basePrueba(t)
	s, _ := CrearSesion(base, "A", "dibujo", nil)

	id, err := EncolarOp(base, s.ID, `[{"accion":"borrar","ids":["a"]}]`)
	if err != nil {
		t.Fatalf("EncolarOp: %v", err)
	}
	pend, err := OpsPendientes(base, s.ID)
	if err != nil || len(pend) != 1 || pend[0].ID != id || pend[0].Op != `[{"accion":"borrar","ids":["a"]}]` {
		t.Fatalf("pendientes = %+v %v", pend, err)
	}

	sid, err := ResponderOp(base, id, true, `{"ids":["a"]}`)
	if err != nil || sid != s.ID {
		t.Fatalf("ResponderOp: sesion=%d err=%v", sid, err)
	}
	var estado, resultado string
	base.QueryRow(`SELECT estado, resultado FROM terminal_pizarra_ops WHERE id = ?`, id).Scan(&estado, &resultado)
	if estado != "aplicada" || resultado != `{"ids":["a"]}` {
		t.Fatalf("fila = %q %q", estado, resultado)
	}
	if pend, _ := OpsPendientes(base, s.ID); len(pend) != 0 {
		t.Fatalf("una op aplicada no se vuelve a entregar: %+v", pend)
	}
	if _, err := ResponderOp(base, id, true, ""); !errors.Is(err, ErrOpNoPendiente) {
		t.Fatalf("segundo ack = %v, debía ser ErrOpNoPendiente", err)
	}
}

func TestOpsFallida(t *testing.T) {
	base := basePrueba(t)
	s, _ := CrearSesion(base, "A", "dibujo", nil)
	id, _ := EncolarOp(base, s.ID, `[]`)
	if _, err := ResponderOp(base, id, false, "falta el id z"); err != nil {
		t.Fatal(err)
	}
	var estado string
	base.QueryRow(`SELECT estado FROM terminal_pizarra_ops WHERE id = ?`, id).Scan(&estado)
	if estado != "fallida" {
		t.Fatalf("estado = %q", estado)
	}
}

func TestOpsBarridoPerezosoVenceAlos30s(t *testing.T) {
	base := basePrueba(t)
	s, _ := CrearSesion(base, "A", "dibujo", nil)
	vieja, _ := EncolarOp(base, s.ID, `[1]`)
	reciente, _ := EncolarOp(base, s.ID, `[2]`)
	base.Exec(`UPDATE terminal_pizarra_ops SET creado = datetime('now','localtime','-31 seconds') WHERE id = ?`, vieja)

	pend, _ := OpsPendientes(base, s.ID)
	if len(pend) != 1 || pend[0].ID != reciente {
		t.Fatalf("la op de 31 s no se entrega: %+v", pend)
	}
	var estado string
	base.QueryRow(`SELECT estado FROM terminal_pizarra_ops WHERE id = ?`, vieja).Scan(&estado)
	if estado != "vencida" {
		t.Fatalf("estado = %q, debía ser vencida", estado)
	}
	if _, err := ResponderOp(base, vieja, true, ""); !errors.Is(err, ErrOpNoPendiente) {
		t.Fatalf("ack tardío = %v, una vencida nunca se aplica", err)
	}
	if n, _ := ContarOpsPendientes(base, s.ID); n != 1 {
		t.Fatalf("pendientes = %d", n)
	}
}

func TestOpsResponderDesconocida(t *testing.T) {
	base := basePrueba(t)
	if _, err := ResponderOp(base, 999, true, ""); err == nil || errors.Is(err, ErrOpNoPendiente) {
		t.Fatalf("op inexistente debía dar otro error: %v", err)
	}
}

func TestSesionRecientePorActualizado(t *testing.T) {
	base := basePrueba(t)
	if _, ok, _ := SesionReciente(base); ok {
		t.Fatal("sin sesiones no hay reciente")
	}
	a, _ := CrearSesion(base, "A", "dibujo", nil)
	b, _ := CrearSesion(base, "B", "dibujo", nil)
	base.Exec(`UPDATE terminal_sesiones SET actualizado = datetime('now','localtime','-1 hour') WHERE id = ?`, b.ID)
	if s, ok, _ := SesionReciente(base); !ok || s.ID != a.ID {
		t.Fatalf("reciente = %+v", s)
	}
}
