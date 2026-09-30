package agentes

// lectura_test.go — camino feliz de listar/leer, extremo a extremo por
// el protocolo MCP real (tools/list + tools/call), contra un Bythos real.

import (
	"testing"

	"bythos-desktop/archivos"
	"bythos-desktop/db"
)

func TestListarCarpetasHappyPath(t *testing.T) {
	c, base := bythosDePrueba(t)
	if _, err := db.CrearCarpeta(base, "Go Backend"); err != nil {
		t.Fatalf("semilla CrearCarpeta: %v", err)
	}
	sesion := sesionDePrueba(t, c)

	var salida salidaListarCarpetas
	isErr, msg := llamar(t, sesion, "listar_carpetas", nil, &salida)
	if isErr {
		t.Fatalf("listar_carpetas devolvió error: %s", msg)
	}
	if len(salida.Carpetas) != 1 {
		t.Fatalf("esperaba 1 carpeta, salió %d: %+v", len(salida.Carpetas), salida.Carpetas)
	}
	if salida.Carpetas[0].Nombre != "Go Backend" {
		t.Fatalf("nombre inesperado: %+v", salida.Carpetas[0])
	}
	if salida.Carpetas[0].Total != 0 || salida.Carpetas[0].Porcentaje != 0 {
		t.Fatalf("carpeta vacía debía tener total/porcentaje 0: %+v", salida.Carpetas[0])
	}
}

func TestListarYLeerRecursoHappyPath(t *testing.T) {
	c, base := bythosDePrueba(t)
	carpeta, err := db.CrearCarpeta(base, "React")
	if err != nil {
		t.Fatalf("semilla CrearCarpeta: %v", err)
	}
	rec, err := db.Guardar(base, carpeta.ID, "http://ejemplo.com/1", "Intro a React", "", "descripción", "articulo")
	if err != nil {
		t.Fatalf("semilla Guardar: %v", err)
	}

	sesion := sesionDePrueba(t, c)

	var listado salidaListarRecursos
	isErr, msg := llamar(t, sesion, "listar_recursos", map[string]any{"carpeta_id": carpeta.ID}, &listado)
	if isErr {
		t.Fatalf("listar_recursos devolvió error: %s", msg)
	}
	if len(listado.Recursos) != 1 || listado.Recursos[0].ID != rec.ID {
		t.Fatalf("esperaba el recurso semilla, salió: %+v", listado.Recursos)
	}

	var detalle RecursoDetalle
	isErr, msg = llamar(t, sesion, "leer_recurso", map[string]any{"id": rec.ID}, &detalle)
	if isErr {
		t.Fatalf("leer_recurso devolvió error: %s", msg)
	}
	if detalle.Titulo != "Intro a React" || detalle.Carpeta != "React" || detalle.CarpetaID != carpeta.ID {
		t.Fatalf("detalle inesperado: %+v", detalle)
	}
}

// TestLeerRecursoDeArchivoExponeRutaLocal confirma el requisito central
// de Paso 0 para el agente: un recurso de ARCHIVO trae, además de lo
// normal, nombre/mime/tamaño y la ruta ABSOLUTA en esta PC — para que un
// CLI de IA lo abra con sus propias herramientas de lectura de archivos,
// sin pedirle el contenido a Bythos por HTTP. Sembramos directo por db/
// (como crearCarpetaSemilla/guardarRecursoSemilla): no hace falta subir
// un archivo de verdad para probar que el MCP expone bien lo que ya está
// en la fila.
func TestLeerRecursoDeArchivoExponeRutaLocal(t *testing.T) {
	c, base := bythosDePrueba(t)
	carpeta, err := db.CrearCarpeta(base, "PDFs")
	if err != nil {
		t.Fatalf("semilla CrearCarpeta: %v", err)
	}
	a, err := db.CrearArchivo(base, "sha-semilla", "apuntes.pdf", "application/pdf", 12345, "sh/sha-semilla.pdf")
	if err != nil {
		t.Fatalf("semilla CrearArchivo: %v", err)
	}
	rec, err := db.GuardarArchivo(base, carpeta.ID, a.ID, "Apuntes", "pdf")
	if err != nil {
		t.Fatalf("semilla GuardarArchivo: %v", err)
	}

	sesion := sesionDePrueba(t, c)

	rutaEsperada := archivos.Absoluta(a.Ruta)

	var listado salidaListarRecursos
	isErr, msg := llamar(t, sesion, "listar_recursos", map[string]any{"carpeta_id": carpeta.ID}, &listado)
	if isErr {
		t.Fatalf("listar_recursos devolvió error: %s", msg)
	}
	if len(listado.Recursos) != 1 || listado.Recursos[0].Archivo == nil {
		t.Fatalf("esperaba 1 recurso con Archivo: %+v", listado.Recursos)
	}
	if listado.Recursos[0].Archivo.RutaLocal != rutaEsperada {
		t.Fatalf("ruta local mal: %q, quería %q", listado.Recursos[0].Archivo.RutaLocal, rutaEsperada)
	}
	if listado.Recursos[0].Archivo.NombreOriginal != "apuntes.pdf" || listado.Recursos[0].Archivo.Tamano != 12345 {
		t.Fatalf("metadata de archivo mal: %+v", listado.Recursos[0].Archivo)
	}

	var detalle RecursoDetalle
	isErr, msg = llamar(t, sesion, "leer_recurso", map[string]any{"id": rec.ID}, &detalle)
	if isErr {
		t.Fatalf("leer_recurso devolvió error: %s", msg)
	}
	if detalle.Archivo == nil || detalle.Archivo.RutaLocal != rutaEsperada {
		t.Fatalf("leer_recurso debió traer la misma ruta local: %+v", detalle.Archivo)
	}
	if detalle.Tipo != "pdf" {
		t.Fatalf("tipo debió ser pdf: %q", detalle.Tipo)
	}

	// Un recurso de LINK normal sigue sin Archivo (nil), no se rompió
	// nada de lo viejo.
	link, err := db.Guardar(base, carpeta.ID, "http://ejemplo.com", "Un link", "", "", "articulo")
	if err != nil {
		t.Fatalf("semilla Guardar link: %v", err)
	}
	var detalleLink RecursoDetalle
	isErr, msg = llamar(t, sesion, "leer_recurso", map[string]any{"id": link.ID}, &detalleLink)
	if isErr {
		t.Fatalf("leer_recurso (link) devolvió error: %s", msg)
	}
	if detalleLink.Archivo != nil {
		t.Fatalf("un link no debió traer Archivo: %+v", detalleLink.Archivo)
	}
}

func TestLeerRecursoInexistente(t *testing.T) {
	c, _ := bythosDePrueba(t)
	sesion := sesionDePrueba(t, c)

	isErr, msg := llamar(t, sesion, "leer_recurso", map[string]any{"id": 999}, nil)
	if !isErr {
		t.Fatalf("esperaba error para un id inexistente")
	}
	if msg == "" {
		t.Fatalf("el error debía traer un mensaje")
	}
}

func TestVerStatsYProgresoCarpeta(t *testing.T) {
	c, base := bythosDePrueba(t)
	carpeta, _ := db.CrearCarpeta(base, "Rust")
	rec, _ := db.Guardar(base, carpeta.ID, "http://ejemplo.com/2", "Ownership", "", "", "articulo")
	if err := db.ActualizarProgreso(base, rec.ID, 100); err != nil {
		t.Fatalf("semilla ActualizarProgreso: %v", err)
	}

	sesion := sesionDePrueba(t, c)

	var progCarpeta ProgresoSalida
	isErr, msg := llamar(t, sesion, "ver_progreso_carpeta", map[string]any{"carpeta_id": carpeta.ID}, &progCarpeta)
	if isErr {
		t.Fatalf("ver_progreso_carpeta devolvió error: %s", msg)
	}
	if progCarpeta.Total != 1 || progCarpeta.Completados != 1 || progCarpeta.Porcentaje != 100 {
		t.Fatalf("progreso de carpeta inesperado: %+v", progCarpeta)
	}

	var stats ProgresoSalida
	isErr, msg = llamar(t, sesion, "ver_stats", nil, &stats)
	if isErr {
		t.Fatalf("ver_stats devolvió error: %s", msg)
	}
	if stats.Total != 1 || stats.Completados != 1 {
		t.Fatalf("stats generales inesperadas: %+v", stats)
	}
}

func TestVerAgendaYActividad(t *testing.T) {
	c, base := bythosDePrueba(t)
	if _, err := db.CrearAgenda(base, "2026-01-15", "", "", "repasar hooks", nil); err != nil {
		t.Fatalf("semilla CrearAgenda: %v", err)
	}

	sesion := sesionDePrueba(t, c)

	var agenda salidaVerAgenda
	isErr, msg := llamar(t, sesion, "ver_agenda", map[string]any{"desde": "2026-01-01", "hasta": "2026-01-31"}, &agenda)
	if isErr {
		t.Fatalf("ver_agenda devolvió error: %s", msg)
	}
	if len(agenda.Notas) != 1 || agenda.Notas[0].Texto != "repasar hooks" {
		t.Fatalf("agenda inesperada: %+v", agenda.Notas)
	}

	var actividad salidaVerActividad
	isErr, msg = llamar(t, sesion, "ver_actividad", nil, &actividad)
	if isErr {
		t.Fatalf("ver_actividad devolvió error: %s", msg)
	}
	// La agenda NO cuenta como actividad (ver db.ActividadPorDia: solo
	// resources+folders), así que puede venir vacía; solo confirmamos
	// que la llamada no rompe.
	_ = actividad
}

func TestExportarCarpeta(t *testing.T) {
	c, base := bythosDePrueba(t)
	carpeta, _ := db.CrearCarpeta(base, "Docker")
	db.Guardar(base, carpeta.ID, "http://ejemplo.com/3", "Volúmenes", "", "", "articulo")

	sesion := sesionDePrueba(t, c)

	var salida salidaExportarCarpeta
	isErr, msg := llamar(t, sesion, "exportar_carpeta", map[string]any{"carpeta_id": carpeta.ID}, &salida)
	if isErr {
		t.Fatalf("exportar_carpeta devolvió error: %s", msg)
	}
	if salida.Contenido == "" {
		t.Fatalf("esperaba contenido markdown, salió vacío")
	}

	isErr, msg = llamar(t, sesion, "exportar_carpeta", map[string]any{"carpeta_id": carpeta.ID, "formato": "gemini"}, &salida)
	if isErr {
		t.Fatalf("exportar_carpeta (gemini) devolvió error: %s", msg)
	}
}
