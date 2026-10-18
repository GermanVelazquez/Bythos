package terminal

// tamAnillo es el replay que guarda cada sesión (design D6, tabla de límites).
const tamAnillo = 256 << 10

// anillo es un búfer circular que descarta lo más viejo al llenarse. No es
// seguro entre goroutines: el gestor lo protege con el mutex de la sesión.
type anillo struct {
	buf    []byte
	inicio int // posición del byte más viejo
	n      int // bytes guardados
}

func nuevoAnillo(cap int) *anillo { return &anillo{buf: make([]byte, cap)} }

// Escribir agrega p; si no entra, se quedan solo los últimos len(buf) bytes.
func (a *anillo) Escribir(p []byte) {
	c := len(a.buf)
	if len(p) >= c {
		copy(a.buf, p[len(p)-c:])
		a.inicio, a.n = 0, c
		return
	}
	fin := (a.inicio + a.n) % c
	k := copy(a.buf[fin:], p)
	copy(a.buf, p[k:])
	if a.n += len(p); a.n > c {
		a.inicio = (a.inicio + a.n - c) % c
		a.n = c
	}
}

// Snapshot devuelve una copia de lo guardado, del más viejo al más nuevo.
func (a *anillo) Snapshot() []byte {
	out := make([]byte, a.n)
	k := copy(out, a.buf[a.inicio:])
	if k < a.n {
		copy(out[k:], a.buf[:a.n-k])
	}
	return out
}
