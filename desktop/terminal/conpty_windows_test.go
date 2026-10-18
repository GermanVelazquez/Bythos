package terminal

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// envHost activa el modo "proceso anfitrión" de TestMain: el binario de
// test se re-ejecuta a sí mismo para tener un proceso que dueñe un PTY y
// que el test pueda matar con TerminateProcess (ver TestJob_...).
const envHost = "BYTHOS_PTY_HOST"

func TestMain(m *testing.M) {
	if os.Getenv(envHost) == "1" {
		os.Exit(correrHost())
	}
	os.Exit(m.Run())
}

func comspec() string {
	if c := os.Getenv("ComSpec"); c != "" {
		return c
	}
	return `C:\Windows\System32\cmd.exe`
}

// correrHost abre un PTY con un ping largo, avisa por stdout el PID del
// nieto (ping.exe, hijo de cmd.exe) y se queda esperando a que lo maten.
func correrHost() int {
	p, err := NuevoPTY(Config{Comando: comspec(), Args: []string{"/c", "ping", "-n", "60", "127.0.0.1"}})
	if err != nil {
		fmt.Fprintln(os.Stderr, "host:", err)
		return 1
	}
	go io.Copy(io.Discard, p)
	nieto, err := buscarHijo(uint32(p.(interface{ Pid() int }).Pid()), "ping.exe", 10*time.Second)
	if err != nil {
		fmt.Fprintln(os.Stderr, "host:", err)
		return 1
	}
	fmt.Printf("PID=%d\n", nieto)
	// Tope de seguridad: si el test muere sin matarnos, no quedamos vivos.
	time.Sleep(60 * time.Second)
	return 0
}

// buscarHijo espera a que aparezca un proceso con ese nombre cuyo padre es
// pidPadre y devuelve su PID.
func buscarHijo(pidPadre uint32, nombre string, tope time.Duration) (uint32, error) {
	limite := time.Now().Add(tope)
	for time.Now().Before(limite) {
		snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
		if err != nil {
			return 0, err
		}
		pe := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
		for err = windows.Process32First(snap, &pe); err == nil; err = windows.Process32Next(snap, &pe) {
			if pe.ParentProcessID == pidPadre && strings.EqualFold(windows.UTF16ToString(pe.ExeFile[:]), nombre) {
				windows.CloseHandle(snap)
				return pe.ProcessID, nil
			}
		}
		windows.CloseHandle(snap)
		time.Sleep(50 * time.Millisecond)
	}
	return 0, fmt.Errorf("no apareció %s hijo de %d", nombre, pidPadre)
}

// captura acumula lo que sale del PTY desde una goroutine propia (el PTY
// exige que alguien vacíe la salida).
type captura struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func leer(p PTY) *captura {
	c := &captura{}
	go func() {
		b := make([]byte, 4096)
		for {
			n, err := p.Read(b)
			if n > 0 {
				c.mu.Lock()
				c.buf.Write(b[:n])
				c.mu.Unlock()
			}
			if err != nil {
				return
			}
		}
	}()
	return c
}

func (c *captura) contiene(s string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return strings.Contains(c.buf.String(), s)
}

func esperar(cond func() bool, tope time.Duration) bool {
	limite := time.Now().Add(tope)
	for time.Now().Before(limite) {
		if cond() {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return cond()
}

// abrirProceso abre un handle para esperar la muerte de pid sin depender de
// que el PID no se reutilice.
func abrirProceso(t *testing.T, pid uint32) windows.Handle {
	t.Helper()
	h, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_TERMINATE, false, pid)
	if err != nil {
		t.Fatalf("OpenProcess(%d): %v", pid, err)
	}
	t.Cleanup(func() {
		windows.TerminateProcess(h, 1) // por si el test falló y quedó vivo
		windows.CloseHandle(h)
	})
	return h
}

func murio(h windows.Handle, tope time.Duration) bool {
	ev, err := windows.WaitForSingleObject(h, uint32(tope.Milliseconds()))
	return err == nil && ev == windows.WAIT_OBJECT_0
}

func TestConPTY_EcoYCodigoDeSalida(t *testing.T) {
	if testing.Short() {
		t.Skip("ConPTY real: se omite con -short")
	}
	p, err := NuevoPTY(Config{Comando: comspec(), Args: []string{"/c", "echo", "hola"}})
	if err != nil {
		t.Fatalf("NuevoPTY: %v", err)
	}
	defer p.Cerrar()
	salida := leer(p)

	select {
	case codigo := <-p.Fin():
		if codigo != 0 {
			t.Errorf("código de salida = %d, quería 0", codigo)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("el proceso no terminó en 10 s")
	}
	// La salida puede llegar un instante después del código de salida.
	if !esperar(func() bool { return salida.contiene("hola") }, 5*time.Second) {
		t.Errorf("la salida no contiene %q", "hola")
	}
}

func TestConPTY_CerrarNoDejaProceso(t *testing.T) {
	if testing.Short() {
		t.Skip("ConPTY real: se omite con -short")
	}
	p, err := NuevoPTY(Config{Comando: comspec(), Args: []string{"/k"}})
	if err != nil {
		t.Fatalf("NuevoPTY: %v", err)
	}
	leer(p)
	h := abrirProceso(t, uint32(p.(interface{ Pid() int }).Pid()))

	if err := p.Cerrar(); err != nil {
		t.Errorf("Cerrar: %v", err)
	}
	if err := p.Cerrar(); err != nil { // idempotente
		t.Errorf("segundo Cerrar: %v", err)
	}
	if !murio(h, 5*time.Second) {
		t.Error("el proceso sigue vivo después de Cerrar")
	}
	if _, err := p.Write([]byte("x")); err == nil {
		t.Error("Write después de Cerrar debía fallar")
	}
}

// Una lectura después de Cerrar devuelve EOF sin bloquearse (y sin tocar un
// handle ya cerrado).
func TestConPTY_ReadDespuesDeCerrar(t *testing.T) {
	if testing.Short() {
		t.Skip("ConPTY real: se omite con -short")
	}
	p, err := NuevoPTY(Config{Comando: comspec(), Args: []string{"/k"}})
	if err != nil {
		t.Fatalf("NuevoPTY: %v", err)
	}
	p.Cerrar()
	res := make(chan error, 1)
	go func() {
		_, err := p.Read(make([]byte, 16))
		res <- err
	}()
	select {
	case err := <-res:
		if err != io.EOF {
			t.Errorf("Read = %v, quería io.EOF", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Read después de Cerrar se bloqueó")
	}
}

// Cerrar justo después de arrancar (el hijo aún inicializa) no debe
// colgarse ni dejar proceso: es el caso del diálogo 0xc0000142.
func TestConPTY_CerrarInmediato(t *testing.T) {
	if testing.Short() {
		t.Skip("ConPTY real: se omite con -short")
	}
	for i := 0; i < 3; i++ {
		p, err := NuevoPTY(Config{Comando: comspec(), Args: []string{"/k"}})
		if err != nil {
			t.Fatalf("NuevoPTY: %v", err)
		}
		h := abrirProceso(t, uint32(p.(interface{ Pid() int }).Pid()))
		hecho := make(chan struct{})
		go func() { p.Cerrar(); close(hecho) }()
		select {
		case <-hecho:
		case <-time.After(10 * time.Second):
			t.Fatal("Cerrar se colgó")
		}
		if !murio(h, 5*time.Second) {
			t.Error("el proceso sigue vivo después de Cerrar")
		}
	}
}

// Si el anfitrión muere sin llamar a Cerrar (TerminateProcess), el SO cierra
// el handle del Job y KILL_ON_JOB_CLOSE mata al nieto.
func TestJob_KillOnJobClose(t *testing.T) {
	if testing.Short() {
		t.Skip("ConPTY real: se omite con -short")
	}
	host := exec.Command(os.Args[0])
	host.Env = append(os.Environ(), envHost+"=1")
	salida, err := host.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := host.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { host.Process.Kill(); host.Wait() })

	linea := make(chan string, 1)
	go func() {
		s, _ := bufio.NewReader(salida).ReadString('\n')
		linea <- s
	}()
	var pid uint64
	select {
	case s := <-linea:
		pid, err = strconv.ParseUint(strings.TrimSpace(strings.TrimPrefix(s, "PID=")), 10, 32)
		if err != nil {
			t.Fatalf("el anfitrión no informó un PID válido (%q): %v", s, err)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("el anfitrión no informó el PID del nieto")
	}
	nieto := abrirProceso(t, uint32(pid))

	if err := host.Process.Kill(); err != nil { // TerminateProcess
		t.Fatalf("matar anfitrión: %v", err)
	}
	if !murio(nieto, 5*time.Second) {
		t.Error("el nieto sigue vivo 5 s después de matar al anfitrión")
	}
}

func TestConPTY_VersionMinima(t *testing.T) {
	casos := []struct {
		nombre      string
		compilacion uint32
		quiereErr   bool
	}{
		{"Windows 10 1803", 17134, true},
		{"justo debajo", 17762, true},
		{"Windows 10 1809", 17763, false},
		{"Windows 11", 26200, false},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			err := comprobarCompilacion(func() uint32 { return c.compilacion })
			if c.quiereErr && !errors.Is(err, ErrNoSoportado) {
				t.Errorf("err = %v, quería ErrNoSoportado", err)
			}
			if !c.quiereErr && err != nil {
				t.Errorf("err = %v, quería nil", err)
			}
		})
	}
}
