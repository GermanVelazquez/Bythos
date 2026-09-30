package lan

// interfaces.go — Qué interfaz de red usar para bindear el listener LAN.
// filtrarYOrdenar es pura (testeable con una lista falsa); solo
// InterfacesDisponibles toca el SO (net.Interfaces).

import (
	"net"
	"sort"
	"strings"
)

// Interfaz es una candidata: su IP, nombre de adaptador y Rango
// (IP&máscara), que guardia.go usa para el chequeo de subred.
type Interfaz struct {
	IP     net.IP
	Nombre string
	Rango  *net.IPNet
}

// prefijosExcluidos: VPN/virtualización nunca son "la red de tu casa".
var prefijosExcluidos = []string{
	"vethernet", "hyper-v", "wsl", "virtualbox", "vmware",
	"tap", "wireguard", "tailscale", "zerotier",
}

func excluida(nombre string) bool {
	n := strings.ToLower(nombre)
	for _, p := range prefijosExcluidos {
		if strings.Contains(n, p) {
			return true
		}
	}
	return false
}

// rangoPrioridad: 0 Wi-Fi, 1 Ethernet, 2 el resto (Wi-Fi > Ethernet > otro).
func rangoPrioridad(nombre string) int {
	n := strings.ToLower(nombre)
	switch {
	case strings.Contains(n, "wi-fi"), strings.Contains(n, "wifi"), strings.Contains(n, "wlan"):
		return 0
	case strings.Contains(n, "ethernet"), strings.Contains(n, "eth"):
		return 1
	default:
		return 2
	}
}

// esPrivadaRFC1918 descarta públicas y link-local (169.254.x.x).
func esPrivadaRFC1918(ip net.IP) bool {
	ip4 := ip.To4()
	if ip4 == nil {
		return false
	}
	switch {
	case ip4[0] == 10:
		return true
	case ip4[0] == 172 && ip4[1] >= 16 && ip4[1] <= 31:
		return true
	case ip4[0] == 192 && ip4[1] == 168:
		return true
	default:
		return false
	}
}

// candidatoCrudo desacopla la selección de net.Interface para que los
// tests armen listas falsas sin tocar el SO.
type candidatoCrudo struct {
	Nombre  string
	Activa  bool
	IP      net.IP
	Mascara net.IPMask
}

// filtrarYOrdenar: activa + IPv4 RFC1918 + no excluida; ordena por
// rangoPrioridad y luego por nombre (orden estable).
func filtrarYOrdenar(crudas []candidatoCrudo) []Interfaz {
	out := []Interfaz{}
	for _, c := range crudas {
		if !c.Activa || excluida(c.Nombre) {
			continue
		}
		ip := c.IP.To4()
		if ip == nil || !esPrivadaRFC1918(ip) {
			continue
		}
		out = append(out, Interfaz{
			IP:     ip,
			Nombre: c.Nombre,
			Rango:  &net.IPNet{IP: ip.Mask(c.Mascara), Mask: c.Mascara},
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		ri, rj := rangoPrioridad(out[i].Nombre), rangoPrioridad(out[j].Nombre)
		if ri != rj {
			return ri < rj
		}
		return out[i].Nombre < out[j].Nombre
	})
	return out
}

// InterfacesDisponibles lista candidatas reales de la máquina.
func InterfacesDisponibles() ([]Interfaz, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	var crudas []candidatoCrudo
	for _, iface := range ifaces {
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			if ipnet, ok := addr.(*net.IPNet); ok {
				crudas = append(crudas, candidatoCrudo{
					Nombre: iface.Name, Activa: iface.Flags&net.FlagUp != 0,
					IP: ipnet.IP, Mascara: ipnet.Mask,
				})
			}
		}
	}
	return filtrarYOrdenar(crudas), nil
}
