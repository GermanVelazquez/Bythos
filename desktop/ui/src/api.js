// api.js — El TRADUCTOR. El único que sabe hablar con Go.
// Todo React importa desde aquí, nadie hace fetch suelto.
//
// ¿Por qué un solo archivo y no fetch en cada componente?
// Porque si mañana cambias /api/carpetas por /api/v2/carpetas,
// lo cambias UNA vez aquí, no en 10 componentes. Regla senior:
// un solo lugar que conoce la red.

const BASE = '' // vacío = misma máquina, misma puerta.
// En dev (pnpm dev) Vite lo manda a :8080 por el proxy.
// En .exe final, Go sirve UI+API en :8080. Mismo código, cero cambios.

// X-Bythos-Origen: app en TODA petición: así el historial (ver vista
// Historial y db/eventos.go en Go) sabe que el cambio vino de la UI,
// no de la extensión ni de un agente de IA por MCP.
const CABECERAS_ORIGEN = { 'X-Bythos-Origen': 'app' }
const CABECERAS_JSON = { 'Content-Type': 'application/json', ...CABECERAS_ORIGEN }

// leer() es el portero: si Go dice error (400/500), lanza Error en español.
// Sin esto, cada componente repetiría el mismo if (!res.ok).
async function leer(res) {
  const tipo = res.headers.get('content-type') || ''
  const dato = tipo.includes('application/json') ? await res.json() : await res.text()
  if (!res.ok) {
    // Go manda {error: "..."}, lo mostramos tal cual
    throw new Error(dato?.error || `Error ${res.status}`)
  }
  return dato
}

export const api = {
  salud: () =>
    fetch(`${BASE}/api/salud`, { headers: CABECERAS_ORIGEN }).then(leer),

  listarCarpetas: () =>
    fetch(`${BASE}/api/carpetas`, { headers: CABECERAS_ORIGEN }).then(leer),

  crearCarpeta: (nombre) =>
    fetch(`${BASE}/api/carpetas`, {
      method: 'POST',
      headers: CABECERAS_JSON,
      body: JSON.stringify({ nombre }),
    }).then(leer),

  borrarCarpeta: (id) =>
    fetch(`${BASE}/api/carpetas/${id}`, { method: 'DELETE', headers: CABECERAS_ORIGEN }).then(leer),

  listarRecursos: (carpeta_id) => {
    // 0 o vacío = todos (igual que db.ListarRecursos en Go)
    const q = carpeta_id ? `?carpeta_id=${carpeta_id}` : ''
    return fetch(`${BASE}/api/recursos${q}`, { headers: CABECERAS_ORIGEN }).then(leer)
  },

  // EL botón 💾 Guardar habla aquí: url + carpeta_id, nada más.
  // El estado nace pendiente solo en Go, la UI no lo decide.
  guardarRecurso: (url, carpeta_id, extra = {}) =>
    fetch(`${BASE}/api/recursos`, {
      method: 'POST',
      headers: CABECERAS_JSON,
      body: JSON.stringify({ url, carpeta_id, ...extra }),
    }).then(leer),

  cambiarEstado: (id, estado) =>
    fetch(`${BASE}/api/recursos/${id}`, {
      method: 'PATCH',
      headers: CABECERAS_JSON,
      body: JSON.stringify({ estado }),
    }).then(leer),

  actualizarProgreso: (id, progreso) =>
    fetch(`${BASE}/api/recursos/${id}`, {
      method: 'PATCH',
      headers: CABECERAS_JSON,
      body: JSON.stringify({ progreso }),
    }).then(leer),

  borrarRecurso: (id) =>
    fetch(`${BASE}/api/recursos/${id}`, { method: 'DELETE', headers: CABECERAS_ORIGEN }).then(leer),

  // TERMÓMETRO: % por carpeta (barra) y general (gráfico portada).
  // Mismos nombres que Go: progresoCarpeta ↔ /carpetas/{id}/progreso, stats ↔ /stats.
  progresoCarpeta: (id) =>
    fetch(`${BASE}/api/carpetas/${id}/progreso`, { headers: CABECERAS_ORIGEN }).then(leer),

  stats: () =>
    fetch(`${BASE}/api/stats`, { headers: CABECERAS_ORIGEN }).then(leer),

  // REPASO: pide TEXTO (markdown), no JSON.
  // ¿Por qué funciona con el mismo leer()? Porque leer() mira el
  // Content-Type: si es JSON lo parsea, si no devuelve texto tal cual.
  // Un solo portero para 2 formatos. Drive no viene aquí: ese se
  // DESCARGA con window.open (el navegador pide el attachment directo).
  exportar: (carpetaId, formato) =>
    fetch(`${BASE}/api/carpetas/${carpetaId}/export?format=${formato}`, { headers: CABECERAS_ORIGEN }).then(leer),

  importarAvance: (carpetaId, markdown) =>
    fetch(`${BASE}/api/carpetas/${carpetaId}/import-avance`, {
      method: 'POST',
      headers: CABECERAS_JSON,
      body: JSON.stringify({ markdown }),
    }).then(leer),

  // AGENDA: notas del calendario en un rango + historia de creaciones.
  // Mismos nombres que Go: agendaRango ↔ /agenda, actividad ↔ /actividad.
  agendaRango: (desde, hasta) => {
    const q = new URLSearchParams()
    if (desde) q.set('desde', desde)
    if (hasta) q.set('hasta', hasta)
    const s = q.toString()
    return fetch(`${BASE}/api/agenda${s ? `?${s}` : ''}`, { headers: CABECERAS_ORIGEN }).then(leer)
  },

  crearAgenda: (payload) =>
    fetch(`${BASE}/api/agenda`, {
      method: 'POST',
      headers: CABECERAS_JSON,
      body: JSON.stringify(payload),
    }).then(leer),

  borrarAgenda: (id) =>
    fetch(`${BASE}/api/agenda/${id}`, { method: 'DELETE', headers: CABECERAS_ORIGEN }).then(leer),

  // IMPORT MD: pega compromisos en markdown, dry=true solo previsualiza.
  // nombre viaja solo al confirmar (el preview no crea lote).
  // decisiones asigna carpeta por bloque al confirmar:
  // [{bloque (1-based), accion: auto|usar|crear|suelta, carpeta_id?, nombre?}].
  importarAgendaMD: (markdown, dry = false, nombre = '', decisiones = []) => {
    const body = { markdown }
    if (!dry && nombre && nombre.trim()) body.nombre = nombre.trim()
    if (!dry && Array.isArray(decisiones) && decisiones.length > 0) body.decisiones = decisiones
    return fetch(`${BASE}/api/agenda/import${dry ? '?dry=true' : ''}`, {
      method: 'POST',
      headers: CABECERAS_JSON,
      body: JSON.stringify(body),
    }).then(leer)
  },

  // LOTES: el MD importado persiste como fuente editable/eliminable.
  listarLotes: () =>
    fetch(`${BASE}/api/agenda/imports`, { headers: CABECERAS_ORIGEN }).then(leer),

  obtenerLote: (id) =>
    fetch(`${BASE}/api/agenda/imports/${id}`, { headers: CABECERAS_ORIGEN }).then(leer),

  actualizarLote: (id, { nombre, markdown, decisiones }) => {
    const body = { nombre, markdown }
    if (Array.isArray(decisiones) && decisiones.length > 0) body.decisiones = decisiones
    return fetch(`${BASE}/api/agenda/imports/${id}`, {
      method: 'PUT',
      headers: CABECERAS_JSON,
      body: JSON.stringify(body),
    }).then(leer)
  },

  borrarLote: (id) =>
    fetch(`${BASE}/api/agenda/imports/${id}`, { method: 'DELETE', headers: CABECERAS_ORIGEN }).then(leer),

  actividad: (desde, hasta) => {
    const q = new URLSearchParams()
    if (desde) q.set('desde', desde)
    if (hasta) q.set('hasta', hasta)
    const s = q.toString()
    return fetch(`${BASE}/api/actividad${s ? `?${s}` : ''}`, { headers: CABECERAS_ORIGEN }).then(leer)
  },

  // HISTORIAL: quién tocó los datos (app, extensión o un agente de IA
  // por MCP) y qué cambió. Mismos nombres que Go: listarEventos ↔ /eventos.
  listarEventos: (limite, origen) => {
    const q = new URLSearchParams()
    if (limite) q.set('limite', limite)
    if (origen) q.set('origen', origen)
    const s = q.toString()
    return fetch(`${BASE}/api/eventos${s ? `?${s}` : ''}`, { headers: CABECERAS_ORIGEN }).then(leer)
  },
}
