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
    fetch(`${BASE}/api/salud`).then(leer),

  listarCarpetas: () =>
    fetch(`${BASE}/api/carpetas`).then(leer),

  crearCarpeta: (nombre) =>
    fetch(`${BASE}/api/carpetas`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ nombre }),
    }).then(leer),

  borrarCarpeta: (id) =>
    fetch(`${BASE}/api/carpetas/${id}`, { method: 'DELETE' }).then(leer),

  listarRecursos: (carpeta_id) => {
    // 0 o vacío = todos (igual que db.ListarRecursos en Go)
    const q = carpeta_id ? `?carpeta_id=${carpeta_id}` : ''
    return fetch(`${BASE}/api/recursos${q}`).then(leer)
  },

  // EL botón 💾 Guardar habla aquí: url + carpeta_id, nada más.
  // El estado nace pendiente solo en Go, la UI no lo decide.
  guardarRecurso: (url, carpeta_id, extra = {}) =>
    fetch(`${BASE}/api/recursos`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ url, carpeta_id, ...extra }),
    }).then(leer),

  cambiarEstado: (id, estado) =>
    fetch(`${BASE}/api/recursos/${id}`, {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ estado }),
    }).then(leer),

  actualizarProgreso: (id, progreso) =>
    fetch(`${BASE}/api/recursos/${id}`, {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ progreso }),
    }).then(leer),

  borrarRecurso: (id) =>
    fetch(`${BASE}/api/recursos/${id}`, { method: 'DELETE' }).then(leer),

  // TERMÓMETRO: % por carpeta (barra) y general (gráfico portada).
  // Mismos nombres que Go: progresoCarpeta ↔ /carpetas/{id}/progreso, stats ↔ /stats.
  progresoCarpeta: (id) =>
    fetch(`${BASE}/api/carpetas/${id}/progreso`).then(leer),

  stats: () =>
    fetch(`${BASE}/api/stats`).then(leer),

  // REPASO: pide TEXTO (markdown), no JSON.
  // ¿Por qué funciona con el mismo leer()? Porque leer() mira el
  // Content-Type: si es JSON lo parsea, si no devuelve texto tal cual.
  // Un solo portero para 2 formatos. Drive no viene aquí: ese se
  // DESCARGA con window.open (el navegador pide el attachment directo).
  exportar: (carpetaId, formato) =>
    fetch(`${BASE}/api/carpetas/${carpetaId}/export?format=${formato}`).then(leer),

  importarAvance: (carpetaId, markdown) =>
    fetch(`${BASE}/api/carpetas/${carpetaId}/import-avance`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ markdown }),
    }).then(leer),
}
