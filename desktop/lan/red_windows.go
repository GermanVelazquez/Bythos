//go:build windows

package lan

// red_windows.go — Detecta la categoría de red con COM INetworkListManager
// (llamadas crudas por vtable, sin dependencias extra). Se enumeran las
// conexiones de red, se cruza el adaptador de cada una con la IP bindeada
// vía GetAdaptersAddresses, y se lee la categoría de su INetwork.

import (
	"errors"
	"net"
	"runtime"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	clsidNetworkListManager = windows.GUID{Data1: 0xDCB00C01, Data2: 0x570F, Data3: 0x4A9B, Data4: [8]byte{0x8D, 0x69, 0x19, 0x9F, 0xDB, 0xA5, 0x72, 0x3B}}
	iidNetworkListManager   = windows.GUID{Data1: 0xDCB00000, Data2: 0x570F, Data3: 0x4A9B, Data4: [8]byte{0x8D, 0x69, 0x19, 0x9F, 0xDB, 0xA5, 0x72, 0x3B}}
	procCoCreateInstance    = windows.NewLazySystemDLL("ole32.dll").NewProc("CoCreateInstance")
)

// com apunta a un objeto COM; vtable[i] es el método en la posición i.
type com struct{ p uintptr }

func (o com) llamar(i int, args ...uintptr) uintptr {
	// o.p y vtable son direcciones que devolvió COM (memoria fuera del heap
	// de Go); se reinterpretan sin convertir uintptr a Pointer.
	vtable := **(**uintptr)(unsafe.Pointer(&o.p))
	pEntrada := vtable + uintptr(i)*unsafe.Sizeof(uintptr(0))
	fn := **(**uintptr)(unsafe.Pointer(&pEntrada))
	r, _, _ := syscall.SyscallN(fn, append([]uintptr{o.p}, args...)...)
	return r
}

func objeto(p uintptr) com { return com{p} }

func (o com) liberar() { o.llamar(2) } // IUnknown::Release

// Posiciones en la vtable (IUnknown 0-2, IDispatch 3-6, luego los propios).
const (
	vtGetNetworkConnections = 9 // INetworkListManager
	vtEnumNext              = 8 // IEnumNetworkConnections
	vtConnGetNetwork        = 7 // INetworkConnection
	vtConnGetAdapterId      = 12
	vtNetGetCategory        = 18 // INetwork
)

func categoriaRedSO(ip net.IP) (Categoria, error) {
	// COM es por hilo: se fija el hilo y se inicializa MTA en cada llamada.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := windows.CoInitializeEx(0, windows.COINIT_MULTITHREADED); err != nil {
		var errno syscall.Errno
		// S_FALSE (ya inicializado) no es error; RPC_E_CHANGED_MODE tampoco bloquea.
		if !errors.As(err, &errno) || (errno != 1 && errno != 0x80010106) {
			return CategoriaDesconocida, err
		}
	}
	defer windows.CoUninitialize()

	adaptador, err := adaptadorDeIP(ip)
	if err != nil || adaptador == "" {
		return CategoriaDesconocida, err
	}

	var gestor uintptr
	hr, _, _ := procCoCreateInstance.Call(uintptr(unsafe.Pointer(&clsidNetworkListManager)), 0, 1, // CLSCTX_INPROC_SERVER
		uintptr(unsafe.Pointer(&iidNetworkListManager)), uintptr(unsafe.Pointer(&gestor)))
	if hr != 0 || gestor == 0 {
		return CategoriaDesconocida, errors.New("NLM no disponible")
	}
	defer objeto(gestor).liberar()

	var enumP uintptr
	if objeto(gestor).llamar(vtGetNetworkConnections, uintptr(unsafe.Pointer(&enumP))) != 0 || enumP == 0 {
		return CategoriaDesconocida, errors.New("no se pudo enumerar conexiones")
	}
	enum := objeto(enumP)
	defer enum.liberar()

	for {
		var conn, traidas uintptr
		if enum.llamar(vtEnumNext, 1, uintptr(unsafe.Pointer(&conn)), uintptr(unsafe.Pointer(&traidas))) != 0 || traidas == 0 {
			return CategoriaDesconocida, nil
		}
		cat, ok := categoriaDeConexion(objeto(conn), adaptador)
		objeto(conn).liberar()
		if ok {
			return cat, nil
		}
	}
}

// categoriaDeConexion devuelve la categoría si la conexión es del adaptador.
func categoriaDeConexion(conn com, adaptador string) (Categoria, bool) {
	var id windows.GUID
	if conn.llamar(vtConnGetAdapterId, uintptr(unsafe.Pointer(&id))) != 0 || !strings.EqualFold(id.String(), adaptador) {
		return "", false
	}
	var redP uintptr
	if conn.llamar(vtConnGetNetwork, uintptr(unsafe.Pointer(&redP))) != 0 || redP == 0 {
		return CategoriaDesconocida, true
	}
	red := objeto(redP)
	defer red.liberar()
	var cat int32
	if red.llamar(vtNetGetCategory, uintptr(unsafe.Pointer(&cat))) != 0 {
		return CategoriaDesconocida, true
	}
	switch cat {
	case 0:
		return CategoriaPublica, true
	case 1:
		return CategoriaPrivada, true
	case 2:
		return CategoriaDominio, true
	}
	return CategoriaDesconocida, true
}

// adaptadorDeIP devuelve el GUID "{...}" del adaptador que tiene ip, o "".
func adaptadorDeIP(ip net.IP) (string, error) {
	tam := uint32(15000)
	for intento := 0; intento < 3; intento++ {
		buf := make([]byte, tam)
		lista := (*windows.IpAdapterAddresses)(unsafe.Pointer(&buf[0]))
		err := windows.GetAdaptersAddresses(windows.AF_INET, windows.GAA_FLAG_SKIP_ANYCAST|windows.GAA_FLAG_SKIP_MULTICAST|windows.GAA_FLAG_SKIP_DNS_SERVER, 0, lista, &tam)
		if errors.Is(err, windows.ERROR_BUFFER_OVERFLOW) {
			continue
		}
		if err != nil {
			return "", err
		}
		for a := lista; a != nil; a = a.Next {
			for u := a.FirstUnicastAddress; u != nil; u = u.Next {
				if u.Address.IP().Equal(ip) {
					return windows.BytePtrToString(a.AdapterName), nil
				}
			}
		}
		return "", nil
	}
	return "", errors.New("GetAdaptersAddresses sin espacio")
}
