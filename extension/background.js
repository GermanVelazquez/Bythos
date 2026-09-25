// background.js — el service worker que le habla a Go en http://localhost:8080.
// content.js corre en el contexto de la página visitada (ej. youtube.com),
// así que si ÉL hiciera el fetch, su Origin sería ese sitio y desktop/api
// lo rechazaría (conGuardia ya no confía en CORS "*"). Acá, en cambio,
// el Origin es chrome-extension://<id>, que sí está en la lista permitida.
// popup.js sigue haciendo fetch directo porque ya corre como extensión.
const API = 'http://localhost:8080'

// Go structs have no `json:` tags, so keys arrive capitalized (ID, Nombre).
const normCarpeta = (c) => ({
  id: c.ID ?? c.id,
  nombre: c.Nombre ?? c.nombre ?? c.name ?? '',
})

async function listarCarpetas() {
  const res = await fetch(`${API}/api/carpetas`)
  const data = await res.json()
  return (Array.isArray(data) ? data : []).map(normCarpeta)
}

async function guardarRecurso(carpeta_id, url) {
  const res = await fetch(`${API}/api/recursos`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ carpeta_id, url }),
  })
  const data = await res.json().catch(() => ({}))
  return { ok: res.ok, error: data.error }
}

// content.js manda { tipo } y espera una respuesta por sendResponse.
// Los dos handlers devuelven `true`: le dice a Chrome "la respuesta llega
// async, no cierres el canal" (si no, sendResponse llega tarde y se pierde).
chrome.runtime.onMessage.addListener((msg, sender, sendResponse) => {
  if (msg?.tipo === 'listarCarpetas') {
    listarCarpetas()
      .then((carpetas) => sendResponse({ ok: true, carpetas }))
      .catch(() => sendResponse({ ok: false })) // app apagada: fetch no conecta
    return true
  }
  if (msg?.tipo === 'guardarRecurso') {
    guardarRecurso(msg.carpeta_id, msg.url)
      .then((r) => sendResponse(r))
      .catch(() => sendResponse({ ok: false, offline: true })) // app apagada
    return true
  }
  // Mensaje desconocido: no respondemos (no hay canal async que mantener).
})
