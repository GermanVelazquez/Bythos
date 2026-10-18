//go:build !windows

package main

// instancia_other.go — fuera de Windows no hay mutex de sesión: Bythos
// de escritorio solo se distribuye para Windows, así que adquirir es un
// no-op que siempre "gana" (el puerto 8080 sigue siendo la última barrera).

func adquirirInstancia() (liberar func(), yaExiste bool, err error) {
	return func() {}, false, nil
}
