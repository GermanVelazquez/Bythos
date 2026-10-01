//go:build !windows

package lan

import "net"

// categoriaRedSO: fuera de Windows no hay NLM; siempre desconocida.
func categoriaRedSO(net.IP) (Categoria, error) { return CategoriaDesconocida, nil }
