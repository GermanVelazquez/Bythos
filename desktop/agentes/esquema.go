package agentes

// esquema.go — Un solo helper: fijar el "enum" de un campo del esquema de
// entrada de una tool. El SDK infiere el esquema desde el struct Go (usa
// el tag `jsonschema` solo como descripción, no admite listas de valores
// ahí), así que para restringir p.ej. estado a pendiente/en_curso/completado
// hay que tomar el esquema inferido y pisarle el Enum a mano.

import (
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"
)

// esquemaEnum infiere el esquema JSON del tipo In y le fija Enum al campo
// (nombre JSON, no nombre Go) indicado. Devuelve el *jsonschema.Schema
// listo para poner en mcp.Tool.InputSchema.
func esquemaEnum[In any](campoJSON string, valores ...string) (*jsonschema.Schema, error) {
	esquema, err := jsonschema.For[In](nil)
	if err != nil {
		return nil, fmt.Errorf("no se pudo inferir el esquema: %w", err)
	}
	propiedad, ok := esquema.Properties[campoJSON]
	if !ok {
		return nil, fmt.Errorf("el esquema no tiene un campo %q", campoJSON)
	}
	enum := make([]any, len(valores))
	for i, v := range valores {
		enum[i] = v
	}
	propiedad.Enum = enum
	return esquema, nil
}
