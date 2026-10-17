package agentes

// tipos.go — Formas de datos, en dos capas:
//   - wire*: lo que Bythos manda por HTTP. db.Carpeta/db.Recurso/db.Agenda
//     y db.Progreso NO llevan json tags (a propósito, ver comentarios en
//     desktop/db/): sus claves viajan Capitalizadas ("ID", "Nombre"...).
//     json.Unmarshal empareja por nombre sin importar mayúsculas, así que
//     estos mirrors con los mismos nombres de campo alcanzan sin tags.
//   - *Salida: lo que este servidor MCP le muestra al agente. Claves en
//     minúscula con snake_case, como espera cualquier tool JSON normal
//     (no el estilo interno de Bythos, que es para su propia UI en React).

// --- Formas que llegan de Bythos (wire) ---

type wireCarpeta struct {
	ID     int64
	Nombre string
}

type wireRecurso struct {
	ID          int64
	CarpetaID   int64
	URL         string
	Titulo      string
	Imagen      string
	Descripcion string
	Tipo        string
	Estado      string
	Progreso    int
	// Archivo viaja solo si el recurso es un ARCHIVO (pdf/video/imagen/
	// documento), no un link (ver desktop/api/archivos.go: recursoJSON).
	Archivo *wireArchivo
}

// wireArchivo es lo que Bythos manda para un recurso de ARCHIVO: nombre,
// mime, tamaño y la ruta ABSOLUTA en disco. Este proceso (agentes/) nunca
// abre bythos.db ni toca disco directo (ver comentario del paquete en
// cliente.go): la ruta absoluta tiene que viajar por HTTP porque es la
// única forma en que este proceso puede saberla.
type wireArchivo struct {
	ID             int64
	NombreOriginal string
	Mime           string
	Tamano         int64
	RutaLocal      string
}

type wireAgenda struct {
	ID            int64
	Fecha         string
	HoraInicio    string
	HoraFin       string
	Texto         string
	CarpetaID     *int64
	CarpetaNombre string
}

type wireProgreso struct {
	Total       int
	Completados int
	Pendientes  int
	EnCurso     int
	Porcentaje  int
	Promedio    int
}

type wireActividad struct {
	Fecha string
	Total int
}

// wireEvento SÍ lleva json tags: a diferencia de carpetas/recursos/agenda,
// GET /api/eventos ya responde snake_case en minúscula (ver listarEventos
// en api/server.go), nace después y no tiene UI vieja que espere
// Capitalizado.
type wireEvento struct {
	ID        int64  `json:"id"`
	Creado    string `json:"creado"`
	Origen    string `json:"origen"`
	Actor     string `json:"actor"`
	Accion    string `json:"accion"`
	EntidadID int64  `json:"entidad_id"`
	Detalle   string `json:"detalle"`
}

// --- Formas que este servidor le devuelve al agente (salida limpia) ---

// CarpetaSalida es una carpeta recién creada o listada sin progreso.
type CarpetaSalida struct {
	ID     int64  `json:"id" jsonschema:"id de la carpeta"`
	Nombre string `json:"nombre" jsonschema:"nombre de la carpeta"`
}

// CarpetaConProgreso es una fila de listar_carpetas: carpeta + termómetro,
// para no obligar al agente a llamar ver_progreso_carpeta una por una.
type CarpetaConProgreso struct {
	ID          int64  `json:"id" jsonschema:"id de la carpeta"`
	Nombre      string `json:"nombre" jsonschema:"nombre de la carpeta"`
	Total       int    `json:"total" jsonschema:"cantidad de recursos en la carpeta"`
	Completados int    `json:"completados" jsonschema:"recursos en estado completado"`
	EnCurso     int    `json:"en_curso" jsonschema:"recursos en estado en_curso"`
	Pendientes  int    `json:"pendientes" jsonschema:"recursos en estado pendiente"`
	Porcentaje  int    `json:"porcentaje" jsonschema:"0 a 100, completados/total"`
}

// ArchivoSalida es el detalle de un recurso de ARCHIVO (a diferencia de
// un link): nombre, tamaño y la ruta absoluta en ESTA PC para que un
// agente de IA en terminal lo abra con sus propias herramientas de
// lectura de archivos (ej. un PDF), sin tener que pedirle el contenido a
// Bythos por HTTP.
type ArchivoSalida struct {
	ID             int64  `json:"id" jsonschema:"id del archivo"`
	NombreOriginal string `json:"nombre_original" jsonschema:"nombre del archivo tal como lo subió el usuario"`
	Mime           string `json:"mime" jsonschema:"tipo MIME detectado por contenido, ej. application/pdf"`
	Tamano         int64  `json:"tamano" jsonschema:"tamaño en bytes"`
	RutaLocal      string `json:"ruta_local" jsonschema:"ruta ABSOLUTA del archivo en el disco de esta PC, lista para abrir con herramientas de lectura de archivos"`
}

func aArchivoSalida(w *wireArchivo) *ArchivoSalida {
	if w == nil {
		return nil
	}
	return &ArchivoSalida{
		ID:             w.ID,
		NombreOriginal: w.NombreOriginal,
		Mime:           w.Mime,
		Tamano:         w.Tamano,
		RutaLocal:      w.RutaLocal,
	}
}

// RecursoSalida es un recurso guardado en Bythos: un link, o un archivo
// (PDF/video/imagen/documento) — en ese caso Archivo trae el detalle
// para abrirlo desde el agente, y Archivo es nil para un link normal.
type RecursoSalida struct {
	ID          int64          `json:"id" jsonschema:"id del recurso"`
	CarpetaID   int64          `json:"carpeta_id" jsonschema:"id de la carpeta que lo contiene"`
	URL         string         `json:"url" jsonschema:"URL del recurso; para un ARCHIVO es una forma interna \"archivo:<id>\", no una URL real — usá Archivo.ruta_local en su lugar"`
	Titulo      string         `json:"titulo" jsonschema:"título del recurso"`
	Imagen      string         `json:"imagen" jsonschema:"URL de la miniatura, si Bythos la encontró"`
	Descripcion string         `json:"descripcion" jsonschema:"descripción corta del recurso"`
	Tipo        string         `json:"tipo" jsonschema:"youtube, articulo, otro, pdf, video, imagen o documento"`
	Estado      string         `json:"estado" jsonschema:"pendiente, en_curso o completado"`
	Progreso    int            `json:"progreso" jsonschema:"0 a 100, porcentaje estudiado"`
	Archivo     *ArchivoSalida `json:"archivo,omitempty" jsonschema:"presente solo si el recurso es un archivo subido (no un link)"`
}

// RecursoDetalle es la forma de leer_recurso: igual que RecursoSalida más
// el nombre de la carpeta (sin que el agente tenga que cruzarlo a mano).
type RecursoDetalle struct {
	ID          int64          `json:"id" jsonschema:"id del recurso"`
	URL         string         `json:"url" jsonschema:"URL del recurso; para un ARCHIVO es una forma interna \"archivo:<id>\", no una URL real — usá Archivo.ruta_local en su lugar"`
	Titulo      string         `json:"titulo" jsonschema:"título del recurso"`
	Tipo        string         `json:"tipo" jsonschema:"youtube, articulo, otro, pdf, video, imagen o documento"`
	Descripcion string         `json:"descripcion" jsonschema:"descripción corta del recurso"`
	Imagen      string         `json:"imagen" jsonschema:"URL de la miniatura, si Bythos la encontró"`
	Progreso    int            `json:"progreso" jsonschema:"0 a 100, porcentaje estudiado"`
	Estado      string         `json:"estado" jsonschema:"pendiente, en_curso o completado"`
	CarpetaID   int64          `json:"carpeta_id" jsonschema:"id de la carpeta que lo contiene"`
	Carpeta     string         `json:"carpeta" jsonschema:"nombre de la carpeta que lo contiene"`
	Archivo     *ArchivoSalida `json:"archivo,omitempty" jsonschema:"presente solo si el recurso es un archivo subido (no un link)"`
}

// AgendaSalida es una nota del calendario.
type AgendaSalida struct {
	ID            int64  `json:"id" jsonschema:"id de la nota"`
	Fecha         string `json:"fecha" jsonschema:"YYYY-MM-DD"`
	HoraInicio    string `json:"hora_inicio,omitempty" jsonschema:"HH:MM, vacío si no tiene hora"`
	HoraFin       string `json:"hora_fin,omitempty" jsonschema:"HH:MM, vacío si no tiene hora"`
	Texto         string `json:"texto" jsonschema:"texto de la nota"`
	CarpetaID     *int64 `json:"carpeta_id,omitempty" jsonschema:"id de la carpeta asociada, si tiene"`
	CarpetaNombre string `json:"carpeta_nombre,omitempty" jsonschema:"nombre de la carpeta asociada, si tiene"`
}

// ProgresoSalida es el termómetro: de una carpeta (ver_progreso_carpeta)
// o general (ver_stats), misma forma en los dos casos.
type ProgresoSalida struct {
	Total       int `json:"total" jsonschema:"cantidad total de recursos"`
	Completados int `json:"completados" jsonschema:"recursos en estado completado"`
	EnCurso     int `json:"en_curso" jsonschema:"recursos en estado en_curso"`
	Pendientes  int `json:"pendientes" jsonschema:"recursos en estado pendiente"`
	Porcentaje  int `json:"porcentaje" jsonschema:"0 a 100, completados/total"`
	Promedio    int `json:"promedio" jsonschema:"0 a 100, promedio de progreso de todos los recursos"`
}

// ActividadSalida es un punto de historia: cuántas cosas se crearon ese día.
type ActividadSalida struct {
	Fecha string `json:"fecha" jsonschema:"YYYY-MM-DD"`
	Total int    `json:"total" jsonschema:"carpetas + recursos creados ese día"`
}

// EventoSalida es una fila del historial de cambios: quién hizo qué y cuándo.
type EventoSalida struct {
	ID        int64  `json:"id" jsonschema:"id del evento"`
	Creado    string `json:"creado" jsonschema:"fecha y hora local del cambio"`
	Origen    string `json:"origen" jsonschema:"app, extension, agente o desconocido"`
	Actor     string `json:"actor,omitempty" jsonschema:"nombre de quien hizo el cambio (ej. el cliente MCP), vacío si no se identificó"`
	Accion    string `json:"accion" jsonschema:"qué se hizo, ej. carpeta_creada, progreso_cambiado"`
	EntidadID int64  `json:"entidad_id" jsonschema:"id de la carpeta/recurso/nota/lote afectado"`
	Detalle   string `json:"detalle" jsonschema:"descripción legible del cambio, ej. \"Hooks\": 40% → 80%"`
}

func aProgresoSalida(w wireProgreso) ProgresoSalida {
	return ProgresoSalida{
		Total:       w.Total,
		Completados: w.Completados,
		EnCurso:     w.EnCurso,
		Pendientes:  w.Pendientes,
		Porcentaje:  w.Porcentaje,
		Promedio:    w.Promedio,
	}
}

func aRecursoSalida(w wireRecurso) RecursoSalida {
	return RecursoSalida{
		ID:          w.ID,
		CarpetaID:   w.CarpetaID,
		URL:         w.URL,
		Titulo:      w.Titulo,
		Imagen:      w.Imagen,
		Descripcion: w.Descripcion,
		Tipo:        w.Tipo,
		Estado:      w.Estado,
		Progreso:    w.Progreso,
		Archivo:     aArchivoSalida(w.Archivo),
	}
}

func aAgendaSalida(w wireAgenda) AgendaSalida {
	return AgendaSalida{
		ID:            w.ID,
		Fecha:         w.Fecha,
		HoraInicio:    w.HoraInicio,
		HoraFin:       w.HoraFin,
		Texto:         w.Texto,
		CarpetaID:     w.CarpetaID,
		CarpetaNombre: w.CarpetaNombre,
	}
}

func aActividadSalida(w wireActividad) ActividadSalida {
	return ActividadSalida{Fecha: w.Fecha, Total: w.Total}
}

func aEventoSalida(w wireEvento) EventoSalida {
	return EventoSalida{
		ID:        w.ID,
		Creado:    w.Creado,
		Origen:    w.Origen,
		Actor:     w.Actor,
		Accion:    w.Accion,
		EntidadID: w.EntidadID,
		Detalle:   w.Detalle,
	}
}
