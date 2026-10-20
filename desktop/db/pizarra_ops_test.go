package db

import (
	"database/sql"
	"errors"
	"sync"
	"sync/atomic"
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

// --- historial del agente (inversa, retención, cambios) ---

func columnasOps(t *testing.T, base *sql.DB) map[string]bool {
	t.Helper()
	rows, err := base.Query(`PRAGMA table_info(terminal_pizarra_ops)`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var cid, notnull, pk int
		var nombre, tipo string
		var dflt interface{}
		rows.Scan(&cid, &nombre, &tipo, &notnull, &dflt, &pk)
		out[nombre] = true
	}
	return out
}

func TestMigrarOpsHistorialAgregaColumnasEIdempotente(t *testing.T) {
	base := basePrueba(t)
	// Tabla vieja: sin las columnas nuevas, con una fila que debe sobrevivir.
	base.Exec(`DROP TABLE terminal_pizarra_ops`)
	base.Exec(`CREATE TABLE terminal_pizarra_ops (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		sesion_id INTEGER NOT NULL REFERENCES terminal_sesiones(id) ON DELETE CASCADE,
		op TEXT NOT NULL DEFAULT '',
		estado TEXT NOT NULL DEFAULT 'pendiente' CHECK(estado IN ('pendiente','aplicada','fallida','vencida')),
		resultado TEXT NOT NULL DEFAULT '',
		creado TEXT NOT NULL DEFAULT (datetime('now','localtime')),
		aplicado TEXT NULL
	)`)
	s, _ := CrearSesion(base, "A", "dibujo", nil)
	base.Exec(`INSERT INTO terminal_pizarra_ops (sesion_id, op) VALUES (?, '[]')`, s.ID)
	for i := 0; i < 2; i++ { // dos veces: idempotente
		if err := crearTablaOps(base); err != nil {
			t.Fatalf("crearTablaOps #%d: %v", i, err)
		}
	}
	cols := columnasOps(t, base)
	for _, c := range []string{"inversa", "versiones", "deshecho"} {
		if !cols[c] {
			t.Errorf("falta la columna %s", c)
		}
	}
	var n int
	base.QueryRow(`SELECT COUNT(*) FROM terminal_pizarra_ops`).Scan(&n)
	if n != 1 {
		t.Fatalf("la fila vieja debía sobrevivir: %d", n)
	}
}

func TestResponderOpGuardaInversaYVersiones(t *testing.T) {
	base := basePrueba(t)
	s, _ := CrearSesion(base, "A", "dibujo", nil)
	id, _ := EncolarOp(base, s.ID, `[{"accion":"borrar","ids":["a"]}]`)
	inv, ver := `{"restaurar":[],"quitar":["a"]}`, `{"a":[1,7]}`
	sid, op, err := ResponderOpHistorial(base, id, true, `{"ids":["a"]}`, &inv, &ver)
	if err != nil || sid != s.ID || op != `[{"accion":"borrar","ids":["a"]}]` {
		t.Fatalf("sid=%d op=%q err=%v", sid, op, err)
	}
	var gi, gv sql.NullString
	base.QueryRow(`SELECT inversa, versiones FROM terminal_pizarra_ops WHERE id = ?`, id).Scan(&gi, &gv)
	if gi.String != inv || gv.String != ver {
		t.Fatalf("guardado = %q %q", gi.String, gv.String)
	}
}

func TestResponderOpSinInversaGuardaNull(t *testing.T) {
	base := basePrueba(t)
	s, _ := CrearSesion(base, "A", "dibujo", nil)
	id, _ := EncolarOp(base, s.ID, `[]`)
	if _, err := ResponderOp(base, id, true, ""); err != nil {
		t.Fatal(err)
	}
	var gi sql.NullString
	base.QueryRow(`SELECT inversa FROM terminal_pizarra_ops WHERE id = ?`, id).Scan(&gi)
	if gi.Valid {
		t.Fatalf("un ack sin inversa guarda NULL: %q", gi.String)
	}
}

func aplicarOp(t *testing.T, base *sql.DB, sesion int64, conInversa bool) int64 {
	t.Helper()
	id, _ := EncolarOp(base, sesion, `[{"accion":"borrar","ids":["a"]}]`)
	var inv *string
	if conInversa {
		x := `{"restaurar":[],"quitar":["a"]}`
		inv = &x
	}
	if _, _, err := ResponderOpHistorial(base, id, true, "", inv, nil); err != nil {
		t.Fatal(err)
	}
	return id
}

func existeOp(base *sql.DB, id int64) bool {
	var n int
	base.QueryRow(`SELECT COUNT(*) FROM terminal_pizarra_ops WHERE id = ?`, id).Scan(&n)
	return n == 1
}

func TestRetencionDosTopesIndependientes(t *testing.T) {
	base := basePrueba(t)
	s, _ := CrearSesion(base, "A", "dibujo", nil)
	otra, _ := CrearSesion(base, "B", "dibujo", nil)
	primeraDeshacible := aplicarOp(t, base, s.ID, true)
	primeraSin := aplicarOp(t, base, s.ID, false)
	ajena := aplicarOp(t, base, otra.ID, true)
	pendiente, _ := EncolarOp(base, s.ID, `[]`)
	for i := 0; i < 55; i++ {
		aplicarOp(t, base, s.ID, false) // no desalojan a las deshacibles
	}
	for i := 0; i < 49; i++ {
		aplicarOp(t, base, s.ID, true)
	}
	if !existeOp(base, primeraDeshacible) {
		t.Fatal("los lotes sin deshacer no desalojan a los deshacibles")
	}
	aplicarOp(t, base, s.ID, true) // la 51ª con inversa: la primera sale
	if existeOp(base, primeraDeshacible) {
		t.Fatal("la deshacible más vieja debía podarse al pasar de 50")
	}
	if existeOp(base, primeraSin) {
		t.Fatal("la sin-deshacer más vieja debía podarse (tope propio de 50)")
	}
	if !existeOp(base, ajena) || !existeOp(base, pendiente) {
		t.Fatal("no se poda otra sesión ni una pendiente")
	}
	var conInv, sinInv int
	base.QueryRow(`SELECT COUNT(*) FROM terminal_pizarra_ops WHERE sesion_id = ? AND inversa IS NOT NULL`, s.ID).Scan(&conInv)
	base.QueryRow(`SELECT COUNT(*) FROM terminal_pizarra_ops WHERE sesion_id = ? AND inversa IS NULL AND estado = 'aplicada'`, s.ID).Scan(&sinInv)
	if conInv != 50 || sinInv != 50 {
		t.Fatalf("con inversa=%d sin inversa=%d, ambos debían ser 50", conInv, sinInv)
	}
}

func TestListarCambiosEstadosYOcultas(t *testing.T) {
	base := basePrueba(t)
	s, _ := CrearSesion(base, "A", "dibujo", nil)
	deshacible := aplicarOp(t, base, s.ID, true)
	deshecho := aplicarOp(t, base, s.ID, true)
	base.Exec(`UPDATE terminal_pizarra_ops SET deshecho = datetime('now') WHERE id = ?`, deshecho)
	sin := aplicarOp(t, base, s.ID, false)
	marcas := `[{"accion":"agregar","elemento":{"id":"marca-x-m","tipo":"rectangle"}},{"accion":"agregar","elemento":{"id":"marca-x-t","tipo":"text"}}]`
	idMarcas, _ := EncolarOp(base, s.ID, marcas)
	x := `{"restaurar":[],"quitar":["marca-x-m","marca-x-t"]}`
	ResponderOpHistorial(base, idMarcas, true, "", &x, nil)
	venc, _ := EncolarOp(base, s.ID, `[]`)
	base.Exec(`UPDATE terminal_pizarra_ops SET creado = datetime('now','localtime','-31 seconds') WHERE id = ?`, venc)
	fall, _ := EncolarOp(base, s.ID, `[]`)
	ResponderOp(base, fall, false, "error")
	EncolarOp(base, s.ID, `[]`) // pendiente: oculta

	lista, err := ListarCambios(base, s.ID)
	if err != nil {
		t.Fatal(err)
	}
	got := map[int64]Cambio{}
	for _, c := range lista {
		got[c.ID] = c
	}
	quiero := map[int64]string{deshacible: "deshacible", deshecho: "deshecho", sin: "sin_deshacer", idMarcas: "deshacible", venc: "sin_deshacer"}
	if len(lista) != len(quiero) {
		t.Fatalf("lista = %+v, la fallida y la pendiente no se listan", lista)
	}
	for id, est := range quiero {
		if got[id].Estado != est {
			t.Errorf("op %d: estado %q, quería %q", id, got[id].Estado, est)
		}
	}
	if c := got[idMarcas]; c.Tipo != "marcas" || c.Elementos != 2 {
		t.Errorf("marcas = %+v", c)
	}
	if c := got[deshacible]; c.Tipo != "dibujo" || c.Elementos != 1 || c.Creado == "" {
		t.Errorf("dibujo = %+v", c)
	}
	for i := 1; i < len(lista); i++ {
		if lista[i-1].ID < lista[i].ID {
			t.Fatal("más nuevo primero")
		}
	}
}

func TestReclamarDeshacerEsAtomico(t *testing.T) {
	base := basePrueba(t)
	s, _ := CrearSesion(base, "A", "dibujo", nil)
	id, _ := EncolarOp(base, s.ID, `[]`)
	inv, ver := `{"quitar":["a"]}`, `{"a":[1,1]}`
	if _, _, err := ResponderOpHistorial(base, id, true, "", &inv, &ver); err != nil {
		t.Fatal(err)
	}
	const n = 8
	var wg sync.WaitGroup
	var ganadores, rechazados int32
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, err := ReclamarDeshacer(base, s.ID, id)
			switch {
			case err == nil:
				atomic.AddInt32(&ganadores, 1)
			case errors.Is(err, ErrNoDeshacible):
				atomic.AddInt32(&rechazados, 1)
			default:
				t.Errorf("error inesperado: %v", err)
			}
		}()
	}
	wg.Wait()
	if ganadores != 1 || rechazados != n-1 {
		t.Fatalf("ganadores=%d rechazados=%d, debía ser 1 y %d", ganadores, rechazados, n-1)
	}
}

func TestReclamarDeshacerRechazos(t *testing.T) {
	base := basePrueba(t)
	s, _ := CrearSesion(base, "A", "dibujo", nil)
	otra, _ := CrearSesion(base, "B", "dibujo", nil)
	sinInv, _ := EncolarOp(base, s.ID, `[]`)
	ResponderOp(base, sinInv, true, "")
	if _, _, err := ReclamarDeshacer(base, s.ID, sinInv); !errors.Is(err, ErrNoDeshacible) {
		t.Errorf("sin inversa = %v", err)
	}
	if _, _, err := ReclamarDeshacer(base, s.ID, 999); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("desconocida = %v", err)
	}
	if _, _, err := ReclamarDeshacer(base, otra.ID, sinInv); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("otra sesión = %v", err)
	}
}

func TestLiberarDeshacerEsIdempotente(t *testing.T) {
	base := basePrueba(t)
	s, _ := CrearSesion(base, "A", "dibujo", nil)
	id, _ := EncolarOp(base, s.ID, `[]`)
	inv := `{"quitar":["a"]}`
	ResponderOpHistorial(base, id, true, "", &inv, nil)
	ReclamarDeshacer(base, s.ID, id)
	if lib, err := LiberarDeshacer(base, s.ID, id); err != nil || !lib {
		t.Fatalf("primera liberación = %v %v", lib, err)
	}
	if lib, err := LiberarDeshacer(base, s.ID, id); err != nil || lib {
		t.Fatalf("segunda liberación = %v %v, debía ser idempotente", lib, err)
	}
	if _, _, err := ReclamarDeshacer(base, s.ID, id); err != nil {
		t.Fatalf("tras liberar debía poder reclamarse: %v", err)
	}
	if _, err := LiberarDeshacer(base, s.ID, 999); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("liberar desconocida = %v", err)
	}
}
