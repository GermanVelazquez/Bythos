//go:build windows

package terminal

import (
	"errors"
	"io"
	"runtime"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// compilacionMinimaConPTY es la primera compilación de Windows con ConPTY
// (Windows 10 1809).
const compilacionMinimaConPTY = 17763

// topeLectura es lo máximo que Cerrar espera a que la salida llegue a EOF
// antes de cortar la lectura a la fuerza.
const topeLectura = 2 * time.Second

// compilacionWindows es la compilación del SO; es variable para que los
// tests puedan probar la compuerta sin depender de la máquina.
var compilacionWindows = func() uint32 { return windows.RtlGetVersion().BuildNumber }

// comprobarCompilacion devuelve ErrNoSoportado si el SO es anterior a ConPTY.
func comprobarCompilacion(compilacion func() uint32) error {
	if compilacion() < compilacionMinimaConPTY {
		return ErrNoSoportado
	}
	return nil
}

// conpty es un PTY real sobre ConPTY. Reglas que NO se negocian (ver
// research Q5 y design "ConPTY lifecycle"):
//   - una goroutine por pipe y la salida siempre se vacía, o ConPTY se
//     bloquea;
//   - se cierran inR/outW apenas se crea el proceso;
//   - el hijo nace suspendido, entra al Job y recién ahí corre;
//   - al cerrar, primero se mata el Job y después se cierra la consola, sin
//     dejar de vaciar la salida (cerrarla con el hijo inicializando abre el
//     diálogo 0xc0000142).
type conpty struct {
	hpc   windows.Handle // la consola; la protege mu
	inW   windows.Handle // entrada del usuario hacia el hijo
	outR  windows.Handle // salida del hijo
	job   windows.Handle
	pid   int
	fin   chan int
	waite chan struct{} // se cierra cuando el waiter terminó

	mu      sync.Mutex // protege hpc y cerrado
	cerrado bool

	escribir sync.Mutex // serializa Write y el cierre de inW
	once     sync.Once
}

// NuevoPTY arranca cfg.Comando dentro de una pseudo-consola. Devuelve
// ErrNoSoportado si el Windows es anterior a la compilación 17763.
func NuevoPTY(cfg Config) (PTY, error) {
	if err := comprobarCompilacion(compilacionWindows); err != nil {
		return nil, err
	}
	if cfg.Comando == "" {
		return nil, errors.New("terminal: falta el comando del PTY")
	}
	cols, filas := cfg.tamano()

	var inR, inW, outR, outW, hpc, job windows.Handle
	ok := false
	defer func() {
		// Solo corre el camino de error: si todo salió bien, lo que quedó
		// abierto pasó a ser del conpty.
		if ok {
			return
		}
		if hpc != 0 {
			windows.ClosePseudoConsole(hpc)
		}
		for _, h := range []*windows.Handle{&inR, &inW, &outR, &outW, &job} {
			cerrarHandle(h)
		}
	}()

	if err := windows.CreatePipe(&inR, &inW, nil, 0); err != nil {
		return nil, err
	}
	if err := windows.CreatePipe(&outR, &outW, nil, 0); err != nil {
		return nil, err
	}
	// Flags 0: nunca PSEUDOCONSOLE_INHERIT_CURSOR (pide una respuesta al
	// cursor y puede colgarse).
	size := windows.Coord{X: int16(cols), Y: int16(filas)}
	if err := windows.CreatePseudoConsole(size, inR, outW, 0, &hpc); err != nil {
		return nil, err
	}

	job, err := nuevoJob()
	if err != nil {
		return nil, err
	}

	attrs, err := windows.NewProcThreadAttributeList(1)
	if err != nil {
		return nil, err
	}
	defer attrs.Delete()
	// El valor del atributo ES el handle de la consola, no un puntero a él.
	if err := attrs.Update(windows.PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE, *(*unsafe.Pointer)(unsafe.Pointer(&hpc)), unsafe.Sizeof(hpc)); err != nil {
		return nil, err
	}

	linea, err := windows.UTF16PtrFromString(windows.ComposeCommandLine(append([]string{cfg.Comando}, cfg.Args...)))
	if err != nil {
		return nil, err
	}
	var dir *uint16
	if cfg.Dir != "" {
		if dir, err = windows.UTF16PtrFromString(cfg.Dir); err != nil {
			return nil, err
		}
	}
	entorno, err := bloqueEntorno(cfg.Env)
	if err != nil {
		return nil, err
	}

	si := windows.StartupInfoEx{ProcThreadAttributeList: attrs.List()}
	si.StartupInfo.Cb = uint32(unsafe.Sizeof(si))
	// USESTDHANDLES con los tres handles en 0: sin esto, el hijo hereda el
	// stdout/stderr del proceso padre y su salida se escapa de la consola
	// (el ConPTY solo recibe lo que se escribe en la consola).
	si.StartupInfo.Flags = windows.STARTF_USESTDHANDLES
	var pi windows.ProcessInformation
	// Suspendido: el hijo no corre hasta estar dentro del Job. Sin heredar
	// handles: el hijo solo recibe la consola.
	flags := uint32(windows.CREATE_SUSPENDED | windows.EXTENDED_STARTUPINFO_PRESENT | windows.CREATE_UNICODE_ENVIRONMENT)
	err = windows.CreateProcess(nil, linea, nil, nil, false, flags, entorno, dir, &si.StartupInfo, &pi)
	runtime.KeepAlive(entorno)
	if err != nil {
		return nil, err
	}
	if err := windows.AssignProcessToJobObject(job, pi.Process); err != nil {
		windows.TerminateProcess(pi.Process, 1)
		windows.CloseHandle(pi.Process)
		windows.CloseHandle(pi.Thread)
		return nil, err
	}
	if _, err := windows.ResumeThread(pi.Thread); err != nil {
		// Ya está en el Job: cerrar el Job (camino de error) lo mata.
		windows.CloseHandle(pi.Process)
		windows.CloseHandle(pi.Thread)
		return nil, err
	}
	windows.CloseHandle(pi.Thread)
	// ConPTY ya duplicó estos dos extremos: dejarlos abiertos impide ver EOF.
	cerrarHandle(&inR)
	cerrarHandle(&outW)

	p := &conpty{
		hpc:   hpc,
		inW:   inW,
		outR:  outR,
		job:   job,
		pid:   int(pi.ProcessId),
		fin:   make(chan int, 1),
		waite: make(chan struct{}),
	}
	ok = true
	go p.esperarFin(pi.Process)
	return p, nil
}

// nuevoJob crea el Job de este PTY: cuando se cierra su último handle (aun
// si bythos.exe muere por TerminateProcess) el SO mata a todo el árbol.
func nuevoJob() (windows.Handle, error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return 0, err
	}
	var info windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
		windows.CloseHandle(job)
		return 0, err
	}
	return job, nil
}

// bloqueEntorno arma el bloque UTF-16 doblemente terminado en NUL que pide
// CreateProcess. nil hereda el entorno del proceso.
func bloqueEntorno(env []string) (*uint16, error) {
	if env == nil {
		return nil, nil
	}
	var b []uint16
	for _, kv := range env {
		u, err := windows.UTF16FromString(kv) // incluye el NUL final
		if err != nil {
			return nil, err
		}
		b = append(b, u...)
	}
	if len(b) == 0 {
		b = append(b, 0)
	}
	b = append(b, 0)
	return &b[0], nil
}

// cerrarHandle cierra *h si está abierto y lo deja en 0.
func cerrarHandle(h *windows.Handle) {
	if *h != 0 {
		windows.CloseHandle(*h)
		*h = 0
	}
}

// esperarFin es la goroutine que espera la muerte del hijo y publica su
// código de salida. Es dueña del handle del proceso.
func (p *conpty) esperarFin(proc windows.Handle) {
	defer close(p.waite)
	defer windows.CloseHandle(proc)
	windows.WaitForSingleObject(proc, windows.INFINITE)
	var codigo uint32
	windows.GetExitCodeProcess(proc, &codigo)
	p.fin <- int(codigo)
	close(p.fin)
}

// Pid es el PID del proceso hijo (raíz del árbol).
func (p *conpty) Pid() int { return p.pid }

// Fin entrega el código de salida del hijo.
func (p *conpty) Fin() <-chan int { return p.fin }

func (p *conpty) estaCerrado() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.cerrado
}

// Read lee la salida del hijo. Devuelve io.EOF cuando la consola se cerró.
func (p *conpty) Read(b []byte) (int, error) {
	var n uint32
	err := windows.ReadFile(p.outR, b, &n, nil)
	if n > 0 {
		return int(n), nil
	}
	if err == nil || err == windows.ERROR_BROKEN_PIPE || p.estaCerrado() {
		return 0, io.EOF
	}
	return 0, err
}

// Write envía la entrada del usuario al hijo.
func (p *conpty) Write(b []byte) (int, error) {
	p.escribir.Lock()
	defer p.escribir.Unlock()
	if p.estaCerrado() {
		return 0, io.ErrClosedPipe
	}
	var total int
	for total < len(b) {
		var n uint32
		if err := windows.WriteFile(p.inW, b[total:], &n, nil); err != nil {
			return total, err
		}
		total += int(n)
	}
	return total, nil
}

// Redimensionar cambia el tamaño de la consola.
func (p *conpty) Redimensionar(cols, filas uint16) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cerrado {
		return io.ErrClosedPipe
	}
	return windows.ResizePseudoConsole(p.hpc, windows.Coord{X: int16(cols), Y: int16(filas)})
}

// Cerrar mata el árbol de procesos y libera todo. Es idempotente.
func (p *conpty) Cerrar() error {
	p.once.Do(p.cerrar)
	return nil
}

func (p *conpty) cerrar() {
	// Desde aquí Write y Redimensionar fallan; así nadie usa los handles
	// mientras se cierran.
	p.mu.Lock()
	p.cerrado = true
	p.mu.Unlock()

	// 1) Matar el árbol ANTES de tocar la consola.
	windows.TerminateJobObject(p.job, 1)

	// 2) Cerrar la consola mientras la salida se sigue vaciando: ClosePseudoConsole
	// puede emitir un último frame y bloquearse si nadie lo lee. El lector
	// del gestor sigue activo, pero este también cubre a quien no lo tenga.
	vaciado := make(chan struct{})
	go func() {
		defer close(vaciado)
		b := make([]byte, 4096)
		for {
			var n uint32
			if err := windows.ReadFile(p.outR, b, &n, nil); err != nil {
				return
			}
		}
	}()
	windows.ClosePseudoConsole(p.hpc)

	// 3) Esperar el EOF de la salida, con tope.
	select {
	case <-vaciado:
	case <-time.After(topeLectura):
		windows.CancelIoEx(p.outR, nil)
		<-vaciado
	}

	// 4) Liberar lo que queda. El handle del proceso es del waiter.
	// No se ponen en 0 los campos: Read los lee sin candado y ya devuelve EOF
	// porque cerrado está en true.
	p.escribir.Lock()
	windows.CloseHandle(p.inW)
	p.escribir.Unlock()
	windows.CloseHandle(p.outR)
	windows.CloseHandle(p.job)
	select {
	case <-p.waite:
	case <-time.After(topeLectura):
	}
}
