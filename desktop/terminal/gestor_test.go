package terminal

import (
	"bytes"
	"encoding/binary"
	"errors"
	"sync"
	"testing"
	"time"
)

// ptyFalso es un PTY en memoria: emitir() entrega bytes al lector del gestor
// y vuelve cuando el lector los recibió.
type ptyFalso struct {
	salida  chan []byte
	fin     chan int
	cerrado chan struct{}
	once    sync.Once

	mu          sync.Mutex
	escrito     []byte
	cols, filas uint16
}

func nuevoPtyFalso() *ptyFalso {
	return &ptyFalso{salida: make(chan []byte), fin: make(chan int, 1), cerrado: make(chan struct{})}
}

func (p *ptyFalso) Read(b []byte) (int, error) {
	select {
	case d := <-p.salida:
		return copy(b, d), nil
	case <-p.cerrado:
		return 0, errors.New("EOF")
	}
}
func (p *ptyFalso) Write(b []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.escrito = append(p.escrito, b...)
	return len(b), nil
}
func (p *ptyFalso) Redimensionar(c, f uint16) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cols, p.filas = c, f
	return nil
}
func (p *ptyFalso) Fin() <-chan int { return p.fin }
func (p *ptyFalso) Cerrar() error   { p.once.Do(func() { close(p.cerrado) }); return nil }
func (p *ptyFalso) emitir(t *testing.T, b []byte) {
	t.Helper()
	select {
	case p.salida <- b:
	case <-time.After(2 * time.Second):
		t.Fatal("el lector del gestor se bloqueó")
	}
}
func (p *ptyFalso) estaCerrado() bool {
	select {
	case <-p.cerrado:
		return true
	default:
		return false
	}
}

type entorno struct {
	g     *Gestor
	ptys  []*ptyFalso
	ahora time.Time
}

func nuevoEntorno(max int) *entorno {
	e := &entorno{ahora: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	e.g = NuevoGestor(Opciones{
		Max:    max,
		Gracia: 10 * time.Minute,
		Ahora:  func() time.Time { return e.ahora },
		Iniciar: func(Config) (PTY, error) {
			p := nuevoPtyFalso()
			e.ptys = append(e.ptys, p)
			return p, nil
		},
	})
	return e
}

func cfgVacia() (Config, error) { return Config{}, nil }

func (e *entorno) adjuntar(t *testing.T, id int64) *Adjunto {
	t.Helper()
	a, err := e.g.Adjuntar(id, cfgVacia, 80, 24)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func eventualmente(t *testing.T, cond func() bool) {
	t.Helper()
	fin := time.Now().Add(2 * time.Second)
	for !cond() {
		if time.Now().After(fin) {
			t.Fatal("la condición no se cumplió a tiempo")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestGestorSnapshotYSuscripcionSonAtomicos(t *testing.T) {
	e := nuevoEntorno(8)
	defer e.g.CerrarTodo()
	primero := e.adjuntar(t, 1)
	p := e.ptys[0]
	const total, antes = 300, 100
	listo := make(chan struct{})
	go func() {
		for i := 0; i < total; i++ {
			b := binary.BigEndian.AppendUint32(nil, uint32(i))
			p.emitir(t, b)
			if i == antes {
				close(listo)
			}
		}
	}()
	<-listo
	segundo := e.adjuntar(t, 1) // reemplaza al primero en medio del flujo
	_ = primero
	got := append([]byte(nil), segundo.Repeticion...)
	for len(got) < total*4 {
		select {
		case c, ok := <-segundo.Salida:
			if !ok {
				t.Fatal("Salida cerrada antes de tiempo")
			}
			got = append(got, c...)
		case <-time.After(2 * time.Second):
			t.Fatalf("faltan bytes: %d de %d", len(got), total*4)
		}
	}
	for i := 0; i < total; i++ {
		if binary.BigEndian.Uint32(got[i*4:]) != uint32(i) {
			t.Fatalf("hueco o duplicado en la posición %d", i)
		}
	}
}

func TestGestorAdjuntoNuevoReemplazaAlViejo(t *testing.T) {
	e := nuevoEntorno(8)
	defer e.g.CerrarTodo()
	viejo := e.adjuntar(t, 1)
	e.ptys[0].emitir(t, []byte("hola"))
	eventualmente(t, func() bool {
		viejo.s.mu.Lock()
		defer viejo.s.mu.Unlock()
		return len(viejo.s.anillo.Snapshot()) == 4
	})
	nuevo := e.adjuntar(t, 1)
	for range viejo.Salida { // drena lo encolado hasta el cierre
	}
	if viejo.Motivo() != CierreReemplazado {
		t.Fatalf("motivo = %d", viejo.Motivo())
	}
	if len(e.ptys) != 1 {
		t.Fatalf("se arrancaron %d PTYs", len(e.ptys))
	}
	if string(nuevo.Repeticion) != "hola" {
		t.Fatalf("repetición = %q", nuevo.Repeticion)
	}
	viejo.Soltar() // no debe tocar al adjunto nuevo
	e.ptys[0].emitir(t, []byte("!"))
	if c := <-nuevo.Salida; string(c) != "!" {
		t.Fatalf("chunk = %q", c)
	}
}

func TestGestorClienteLentoSeCortaYElLectorNoSeBloquea(t *testing.T) {
	e := nuevoEntorno(8)
	defer e.g.CerrarTodo()
	a := e.adjuntar(t, 1)
	for i := 0; i < 300; i++ { // nadie drena a.Salida
		e.ptys[0].emitir(t, []byte("x"))
	}
	eventualmente(t, func() bool { return a.Motivo() == CierreLento })
	n := 0
	for range a.Salida { // el canal se cerró: drena lo encolado
		n++
	}
	if n != 256 {
		t.Fatalf("encolados = %d, quería 256", n)
	}
	if len(e.g.Vivas()) != 1 {
		t.Fatal("la sesión debe seguir viva tras cortar al cliente")
	}
}

func TestGestorTopeDePTYs(t *testing.T) {
	e := nuevoEntorno(2)
	defer e.g.CerrarTodo()
	e.adjuntar(t, 1)
	e.adjuntar(t, 2)
	if _, err := e.g.Adjuntar(3, cfgVacia, 80, 24); !errors.Is(err, ErrLimite) {
		t.Fatalf("err = %v, quería ErrLimite", err)
	}
	if len(e.ptys) != 2 {
		t.Fatalf("se arrancó un PTY de más: %d", len(e.ptys))
	}
	e.adjuntar(t, 1) // reatachar no cuenta contra el tope
}

func TestGestorReaperCierraLosDesatendidosMasDeLaGracia(t *testing.T) {
	e := nuevoEntorno(8)
	defer e.g.CerrarTodo()
	e.adjuntar(t, 1).Soltar()
	e.ahora = e.ahora.Add(8 * time.Minute)
	e.adjuntar(t, 2).Soltar()
	e.adjuntar(t, 3) // sigue adjunto
	e.ahora = e.ahora.Add(3 * time.Minute)
	e.g.barrer()
	if got := e.g.Vivas(); len(got) != 2 || got[0] != 2 || got[1] != 3 {
		t.Fatalf("vivas = %v", got)
	}
	if !e.ptys[0].estaCerrado() || e.ptys[1].estaCerrado() || e.ptys[2].estaCerrado() {
		t.Fatal("solo el PTY 1 debía cerrarse")
	}
}

func TestGestorCerrarEsIdempotente(t *testing.T) {
	e := nuevoEntorno(8)
	defer e.g.CerrarTodo()
	a := e.adjuntar(t, 1)
	for i := 0; i < 2; i++ {
		if err := e.g.Cerrar(1); err != nil {
			t.Fatal(err)
		}
	}
	if err := e.g.Cerrar(99); err != nil {
		t.Fatal(err)
	}
	if !e.ptys[0].estaCerrado() || len(e.g.Vivas()) != 0 {
		t.Fatal("la sesión debía desaparecer")
	}
	if _, ok := <-a.Salida; ok {
		t.Fatal("Salida debía cerrarse")
	}
}

func TestGestorReenviaEntradaYTamano(t *testing.T) {
	e := nuevoEntorno(8)
	defer e.g.CerrarTodo()
	a := e.adjuntar(t, 1)
	if _, err := a.Escribir([]byte("ls\r")); err != nil {
		t.Fatal(err)
	}
	if err := a.Redimensionar(120, 40); err != nil {
		t.Fatal(err)
	}
	p := e.ptys[0]
	p.mu.Lock()
	defer p.mu.Unlock()
	if !bytes.Equal(p.escrito, []byte("ls\r")) || p.cols != 120 || p.filas != 40 {
		t.Fatalf("escrito=%q tam=%dx%d", p.escrito, p.cols, p.filas)
	}
}

func TestGestorFinLlevaElCodigoDeSalida(t *testing.T) {
	e := nuevoEntorno(8)
	defer e.g.CerrarTodo()
	a := e.adjuntar(t, 1)
	e.ptys[0].fin <- 7
	select {
	case c := <-a.Fin:
		if c != 7 {
			t.Fatalf("código = %d", c)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Fin no llegó")
	}
	// Quien se adjunta después de la salida también recibe el código.
	a.Soltar()
	b := e.adjuntar(t, 1)
	select {
	case c := <-b.Fin:
		if c != 7 {
			t.Fatalf("código tardío = %d", c)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Fin tardío no llegó")
	}
}
