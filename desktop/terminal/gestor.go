package terminal

import (
	"errors"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// Límites (design, tabla de límites).
const (
	maxPTYs      = 8
	graciaPTY    = 10 * time.Minute
	chunksSalida = 256 // chunks que aguanta un adjunto antes de cortarlo (D5)
)

// Códigos de cierre de un Adjunto (los usa la API como código de cierre WS).
const (
	CierreReemplazado = 4000 // otro adjunto tomó la sesión
	CierreLento       = 4001 // el cliente no vació Salida a tiempo
)

// ErrLimite indica que ya hay Max PTYs vivos; la API lo traduce a un 429.
var ErrLimite = errors.New("terminal: demasiadas terminales abiertas")

// Opciones configura el Gestor. Los ceros toman los valores por defecto.
type Opciones struct {
	Iniciar func(Config) (PTY, error)
	Max     int
	Gracia  time.Duration
	Ahora   func() time.Time
}

// Gestor mantiene los PTYs vivos por id de sesión. Un PTY sobrevive a la
// ventana (D4): solo se cierra con Cerrar, por el reaper tras Gracia sin
// adjunto, o al salir la app.
type Gestor struct {
	op   Opciones
	mu   sync.Mutex
	ses  map[int64]*sesion
	fin  chan struct{}
	once sync.Once
}

type sesion struct {
	pty   PTY
	ahora func() time.Time

	mu        sync.Mutex
	anillo    *anillo
	adj       *Adjunto
	desde     time.Time // desde cuándo no hay adjunto
	terminado bool
	codigo    int
}

// Adjunto es la conexión única de un cliente a una sesión (D5).
type Adjunto struct {
	// Repeticion es lo ya emitido; el cliente hace term.reset() antes de escribirlo.
	Repeticion []byte
	// Salida entrega la salida nueva; se cierra al soltar, reemplazar, cortar
	// por lento o cerrar la sesión (ver Motivo).
	Salida chan []byte
	// Fin entrega el código de salida del proceso hijo.
	Fin chan int

	s      *sesion
	motivo atomic.Int32
}

// NuevoGestor crea el gestor y arranca su reaper.
func NuevoGestor(op Opciones) *Gestor {
	if op.Max <= 0 {
		op.Max = maxPTYs
	}
	if op.Gracia <= 0 {
		op.Gracia = graciaPTY
	}
	if op.Ahora == nil {
		op.Ahora = time.Now
	}
	g := &Gestor{op: op, ses: map[int64]*sesion{}, fin: make(chan struct{})}
	go func() {
		t := time.NewTicker(op.Gracia / 4)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				g.barrer()
			case <-g.fin:
				return
			}
		}
	}()
	return g
}

// Adjuntar conecta con la sesión id, arrancando su PTY si no existe (cfg solo
// se llama entonces). Un adjunto previo se cierra con CierreReemplazado.
func (g *Gestor) Adjuntar(id int64, cfg func() (Config, error), cols, filas uint16) (*Adjunto, error) {
	g.mu.Lock()
	s := g.ses[id]
	if s == nil {
		if len(g.ses) >= g.op.Max {
			g.mu.Unlock()
			return nil, ErrLimite
		}
		c, err := cfg()
		if err != nil {
			g.mu.Unlock()
			return nil, err
		}
		c.Cols, c.Filas = cols, filas
		p, err := g.op.Iniciar(c)
		if err != nil {
			g.mu.Unlock()
			return nil, err
		}
		s = &sesion{pty: p, anillo: nuevoAnillo(tamAnillo), ahora: g.op.Ahora}
		g.ses[id] = s
		go s.leer()
		go s.esperarFin()
	} else {
		_ = s.pty.Redimensionar(cols, filas)
	}
	g.mu.Unlock()

	s.mu.Lock()
	defer s.mu.Unlock()
	s.soltar(CierreReemplazado, g.op.Ahora())
	a := &Adjunto{
		Repeticion: s.anillo.Snapshot(), // snapshot y suscripción, bajo el mismo mutex
		Salida:     make(chan []byte, chunksSalida),
		Fin:        make(chan int, 1),
		s:          s,
	}
	if s.terminado {
		a.Fin <- s.codigo
	}
	s.adj = a
	return a, nil
}

// leer vacía el PTY hasta EOF. Nunca bloquea en el envío al cliente (D5).
func (s *sesion) leer() {
	buf := make([]byte, 32<<10)
	for {
		n, err := s.pty.Read(buf)
		if n > 0 {
			chunk := append([]byte(nil), buf[:n]...)
			s.mu.Lock()
			s.anillo.Escribir(chunk)
			if a := s.adj; a != nil {
				select {
				case a.Salida <- chunk:
				default:
					s.soltar(CierreLento, s.ahora())
				}
			}
			s.mu.Unlock()
		}
		if err != nil {
			return
		}
	}
}

func (s *sesion) esperarFin() {
	c, ok := <-s.pty.Fin()
	if !ok {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.terminado, s.codigo = true, c
	if s.adj != nil {
		select {
		case s.adj.Fin <- c:
		default:
		}
	}
}

// soltar desconecta al adjunto actual (s.mu tomado) y anota desde cuándo.
func (s *sesion) soltar(motivo int32, ahora time.Time) {
	if a := s.adj; a != nil {
		a.motivo.Store(motivo)
		close(a.Salida)
		s.adj, s.desde = nil, ahora
	}
}

// Motivo es el código de cierre si el gestor cortó este adjunto, o 0.
func (a *Adjunto) Motivo() int { return int(a.motivo.Load()) }

// Escribir manda entrada del usuario al PTY.
func (a *Adjunto) Escribir(p []byte) (int, error) { return a.s.pty.Write(p) }

// Redimensionar cambia el tamaño de la terminal.
func (a *Adjunto) Redimensionar(cols, filas uint16) error {
	return a.s.pty.Redimensionar(cols, filas)
}

// Soltar desconecta este adjunto (la sesión sigue viva); no hace nada si ya
// lo reemplazaron o cortaron. El reaper cuenta la gracia desde aquí.
func (a *Adjunto) Soltar() {
	a.s.mu.Lock()
	defer a.s.mu.Unlock()
	if a.s.adj == a {
		a.s.soltar(0, a.s.ahora())
	}
}

// Cerrar mata el PTY de la sesión id y la olvida. Idempotente; un id
// desconocido no es error.
func (g *Gestor) Cerrar(id int64) error {
	g.mu.Lock()
	s := g.ses[id]
	delete(g.ses, id)
	g.mu.Unlock()
	if s == nil {
		return nil
	}
	return s.cerrar()
}

func (s *sesion) cerrar() error {
	err := s.pty.Cerrar()
	s.mu.Lock()
	s.soltar(0, time.Time{})
	s.mu.Unlock()
	return err
}

// CerrarTodo cierra todas las sesiones y detiene el reaper.
func (g *Gestor) CerrarTodo() {
	g.once.Do(func() { close(g.fin) })
	g.mu.Lock()
	ses := g.ses
	g.ses = map[int64]*sesion{}
	g.mu.Unlock()
	for _, s := range ses {
		_ = s.cerrar()
	}
}

// Vivas lista los ids de sesión con PTY, ordenados.
func (g *Gestor) Vivas() []int64 {
	g.mu.Lock()
	defer g.mu.Unlock()
	ids := make([]int64, 0, len(g.ses))
	for id := range g.ses {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

// barrer cierra las sesiones sin adjunto desde hace Gracia o más.
func (g *Gestor) barrer() {
	ahora := g.op.Ahora()
	g.mu.Lock()
	var viejas []*sesion
	for id, s := range g.ses {
		s.mu.Lock()
		if s.adj == nil && ahora.Sub(s.desde) >= g.op.Gracia {
			viejas = append(viejas, s)
			delete(g.ses, id)
		}
		s.mu.Unlock()
	}
	g.mu.Unlock()
	for _, s := range viejas {
		_ = s.cerrar()
	}
}
