package agentes

// servidor.go — Arma el mcp.Server con todas las herramientas y lo corre
// sobre stdio. Esto es lo único que main.go llama para el modo `mcp`.
//
// Ojo con stdout: el protocolo MCP habla newline-delimited JSON por
// stdin/stdout (ver mcp.StdioTransport). Cualquier fmt.Println/log con
// salida a stdout en este proceso rompería el protocolo. Por eso todo
// log de este paquete (y de main.go en modo mcp) usa el paquete "log"
// estándar, que por default escribe a stderr.

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// NuevoServidor arma el servidor MCP con el cliente HTTP dado. Separado
// de Ejecutar para que los tests puedan armar un servidor con un cliente
// apuntando a un httptest.Server, sin pasar por stdio.
func NuevoServidor(c *Cliente, version string) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{
		Name:    "bythos",
		Version: version,
	}, &mcp.ServerOptions{
		Instructions: "Bythos es un gestor de recursos de estudio (links + progreso + " +
			"agenda). Estas herramientas hablan por HTTP con la app real: si no responden, " +
			"es porque Bythos no está abierto en la PC del usuario.",
	})
	RegistrarLectura(s, c)
	RegistrarEscritura(s, c)
	return s
}

// Ejecutar corre el servidor MCP sobre stdio hasta que el cliente (el
// agente) corta la conexión o ctx se cancela. version es api.Version:
// main.go la pasa para no crear un import de bythos-desktop/api aquí
// (este paquete no toca nada de la app salvo por HTTP).
func Ejecutar(ctx context.Context, version string) error {
	s := NuevoServidor(NuevoCliente(), version)
	if err := s.Run(ctx, &mcp.StdioTransport{}); err != nil {
		return fmt.Errorf("servidor mcp: %w", err)
	}
	return nil
}
