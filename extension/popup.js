// popup.js — El MÚSCULO. Lee la pestaña y la manda a tu PC.
// Habla con el MISMO cerebro que la UI: POST http://localhost:8080/api/recursos.
// Sin servidores, sin cuentas, sin tokens. Tu link nunca sale de tu máquina.

// Todo arranca cuando el popup abre (no antes: los ids no existirían).
document.addEventListener('DOMContentLoaded', async () => {
  const urlEl = document.getElementById('page-url')
  const folderEl = document.getElementById('folder')
  const btn = document.getElementById('guardar')
  const msg = document.getElementById('msg')

  // 1. Leer la pestaña actual. Gracias a "activeTab" no la escribes:
  // Chrome te la da solo porque CLICASTE el botón. Cero copiar-pegar.
  const [tab] = await chrome.tabs.query({ active: true, currentWindow: true })
  urlEl.textContent = tab?.url || 'No se detectó URL'

  // Go structs have no `json:` tags, so keys arrive capitalized (ID, Nombre).
  const normCarpeta = (c) => ({
    id: c.ID ?? c.id,
    nombre: c.Nombre ?? c.nombre ?? c.name ?? '',
  })

  // 2. Listar carpetas por NOMBRE (ya no se escribe el ID a mano).
  // Si la app está apagada o no hay carpetas, el select queda explicando el
  // problema y el botón se deshabilita: compat sin romper el flujo anterior.
  btn.disabled = true
  try {
    const res = await fetch('http://localhost:8080/api/carpetas')
    const data = await res.json()
    const carpetas = (Array.isArray(data) ? data : []).map(normCarpeta)
    folderEl.innerHTML = ''
    if (carpetas.length === 0) {
      const opt = document.createElement('option')
      opt.value = ''
      opt.textContent = 'Crea primero una carpeta en la app'
      folderEl.appendChild(opt)
      msg.textContent = 'Sin carpetas: crea una en la app y reabre el popup.'
    } else {
      for (const c of carpetas) {
        const opt = document.createElement('option')
        opt.value = String(c.id)
        opt.textContent = c.nombre
        folderEl.appendChild(opt)
      }
      // 3. Recordar tu carpeta favorita (storage local de Chrome).
      // ¿Por qué? Elegir cada vez es fricción que mata el hábito.
      const saved = await chrome.storage.local.get(['folder'])
      if (saved.folder && carpetas.some((c) => String(c.id) === String(saved.folder))) {
        folderEl.value = String(saved.folder)
      }
      btn.disabled = false
    }
  } catch {
    // fetch solo truena así si Go está apagado. Mensaje que enseña:
    const opt = document.createElement('option')
    opt.value = ''
    opt.textContent = 'App apagada'
    folderEl.appendChild(opt)
    msg.textContent = '❌ Abre primero tu app Bythos (.exe en :8080).'
  }

  btn.addEventListener('click', async () => {
    const carpeta_id = Number(folderEl.value)
    if (!carpeta_id) {
      msg.textContent = 'Elige una carpeta para guardar.'
      return
    }
    if (!tab?.url) {
      msg.textContent = 'Abre una página web para guardarla.'
      return
    }
    // Guardar preferencia para la próxima (no esperamos, es rápido)
    chrome.storage.local.set({ folder: carpeta_id })

    msg.textContent = 'Guardando…'
    try {
      // MISMO contrato que la UI web: {carpeta_id, url}. Nace pendiente en Go.
      const res = await fetch('http://localhost:8080/api/recursos', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ carpeta_id, url: tab.url }),
      })
      const data = await res.json()
      msg.textContent = res.ok
        ? '✅ Guardado en tu PC.'
        : `❌ ${data.error || 'Carpeta inexistente (créala en la app).'}`
    } catch {
      // fetch solo truena así si Go está apagado. Mensaje que enseña:
      msg.textContent = '❌ Abre primero tu app Bythos (.exe en :8080).'
    }
  })
})
