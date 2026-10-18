package api

// puente_lan.go — El adaptador de lan.Biblioteca: lo que el celular guarda
// pasa por la MISMA secuencia que la UI (metadata SSRF-safe, archivos.Guardar,
// db) y deja el evento con origen "celular" y el nombre del dispositivo como
// actor. lan define el puerto; api lo implementa, así lan no importa api.

import (
	"errors"
	"io"

	"bythos-desktop/archivos"
	"bythos-desktop/db"
	"bythos-desktop/lan"
)

// puenteLAN implementa lan.Biblioteca sobre el Servidor.
type puenteLAN struct {
	s *Servidor
	// metadata es obtenerMetadata (cliente SSRF-safe); se inyecta en tests.
	metadata func(link string) Metadata
}

var _ lan.Biblioteca = (*puenteLAN)(nil)

// NuevoPuenteLAN devuelve la Biblioteca que 5a le pasa a lan.NuevoServicio.
func NuevoPuenteLAN(s *Servidor) lan.Biblioteca {
	return &puenteLAN{s: s, metadata: obtenerMetadata}
}

func (p *puenteLAN) ListarCarpetas() ([]lan.Carpeta, error) {
	carpetas, err := db.ListarCarpetas(p.s.Base)
	if err != nil {
		return nil, err
	}
	out := make([]lan.Carpeta, 0, len(carpetas))
	for _, c := range carpetas {
		out = append(out, lan.Carpeta{ID: c.ID, Nombre: c.Nombre})
	}
	return out, nil
}

func (p *puenteLAN) ExisteCarpeta(id int64) bool { return db.ExisteCarpeta(p.s.Base, id) }

func (p *puenteLAN) GuardarLink(carpetaID int64, url, titulo, actor string) (int64, string, error) {
	if !p.ExisteCarpeta(carpetaID) {
		return 0, "", lan.ErrCarpetaNoExiste
	}
	m := p.metadata(url)
	if titulo == "" {
		titulo = m.Titulo
	}
	rec, err := db.Guardar(p.s.Base, carpetaID, url, titulo, m.Imagen, m.Descripcion, m.Tipo)
	if err != nil {
		if !p.ExisteCarpeta(carpetaID) {
			return 0, "", lan.ErrCarpetaNoExiste
		}
		return 0, "", err
	}
	p.registrar(db.AccionRecursoGuardado, rec, actor)
	return rec.ID, tituloOUrl(rec), nil
}

func (p *puenteLAN) GuardarArchivo(carpetaID int64, r io.Reader, nombre, actor string) (int64, error) {
	if !p.ExisteCarpeta(carpetaID) {
		return 0, lan.ErrCarpetaNoExiste
	}
	g, err := archivos.Guardar(r, nombre)
	if err != nil {
		return 0, err // incluye archivos.ErrTipoNoPermitido: lan lo traduce
	}
	a, err := db.CrearArchivo(p.s.Base, g.SHA256, g.NombreOriginal, g.Mime, g.Tamano, g.RutaRelativa)
	if err != nil {
		return 0, err
	}
	rec, err := db.GuardarArchivo(p.s.Base, carpetaID, a.ID, tituloDeArchivo(g.NombreOriginal), archivos.TipoRecurso(g.Mime))
	if err != nil {
		p.s.limpiarArchivoSiHuerfano(a.ID) // la carpeta se borró en el medio
		if !p.ExisteCarpeta(carpetaID) {
			return 0, lan.ErrCarpetaNoExiste
		}
		return 0, errors.New("no se pudo crear el recurso")
	}
	p.registrar(db.AccionArchivoSubido, rec, actor)
	return rec.ID, nil
}

// registrar anota el evento con origen celular; un fallo de historial nunca
// rompe el guardado (mismo criterio que Servidor.registrarEvento).
func (p *puenteLAN) registrar(accion string, rec db.Recurso, actor string) {
	if err := db.RegistrarEvento(p.s.Base, db.OrigenCelular, actor, accion, rec.ID, tituloOUrl(rec)); err != nil {
		logAvisoArchivo("No se pudo registrar evento de historial:", err)
	}
}
