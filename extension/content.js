// content.js — Bythos floating save button, injected on every page (MV3 content_scripts).
// Same backend contract as popup.js and the desktop UI: GET /api/carpetas, POST /api/recursos.
// Shadow DOM keeps our styles off the host page; no external CSS files (simple .exe/repo).
(() => {
  // Guard against double injection (e.g. SPA navigations re-running the script).
  if (window.__bythosInjected) return
  window.__bythosInjected = true

  const API = 'http://localhost:8080'

  // Go structs have no `json:` tags, so keys arrive capitalized (ID, Nombre).
  const normCarpeta = (c) => ({
    id: c.ID ?? c.id,
    nombre: c.Nombre ?? c.nombre ?? c.name ?? '',
  })

  // Shadow host: a single div; everything else lives inside the shadow root.
  const host = document.createElement('div')
  host.id = 'bythos-root'
  ;(document.body || document.documentElement).appendChild(host)
  const shadow = host.attachShadow({ mode: 'open' })

  // All styles inline here (no extra files). Every selector is bythos- prefixed
  // as a second layer of isolation on top of the shadow DOM.
  const style = document.createElement('style')
  style.textContent = `
    .bythos-fab {
      position: fixed; right: 20px; bottom: 20px; z-index: 2147483647;
      width: 48px; height: 48px; border-radius: 50%;
      background: #4f46e5; color: #fff; border: none; cursor: pointer;
      font: 700 22px/1 system-ui, sans-serif;
      box-shadow: 0 4px 16px rgba(0,0,0,.35);
    }
    .bythos-fab:hover { background: #4338ca; }
    .bythos-panel {
      position: fixed; right: 20px; bottom: 80px; z-index: 2147483647;
      width: 280px; padding: 14px;
      background: #fff; color: #111; border-radius: 12px;
      box-shadow: 0 8px 32px rgba(0,0,0,.35);
      font: 14px/1.4 system-ui, sans-serif;
    }
    .bythos-panel[hidden] { display: none; }
    .bythos-title { font-size: 15px; font-weight: 700; margin: 0 0 8px; }
    .bythos-label { display: block; font-size: 12px; color: #475569; margin: 8px 0 4px; }
    .bythos-select {
      width: 100%; box-sizing: border-box; padding: 8px;
      border: 1px solid #cbd5e1; border-radius: 8px; font-size: 13px;
      background: #fff; color: #111;
    }
    .bythos-msg { font-size: 12px; min-height: 16px; margin: 8px 0 0; color: #475569; }
    .bythos-actions { display: flex; gap: 8px; margin-top: 10px; }
    .bythos-send {
      flex: 1; padding: 9px; background: #4f46e5; color: #fff;
      border: none; border-radius: 8px; cursor: pointer; font-size: 13px; font-weight: 600;
    }
    .bythos-send:hover { background: #4338ca; }
    .bythos-send:disabled { opacity: .5; cursor: not-allowed; }
    .bythos-cancel {
      padding: 9px 12px; background: #f1f5f9; color: #111;
      border: none; border-radius: 8px; cursor: pointer; font-size: 13px;
    }
    .bythos-cancel:hover { background: #e2e8f0; }
  `
  shadow.appendChild(style)

  // Floating button: discreet "B" logo, bottom-right, topmost layer.
  const fab = document.createElement('button')
  fab.className = 'bythos-fab'
  fab.title = 'Guardar en Bythos'
  fab.textContent = 'B'
  shadow.appendChild(fab)

  // Save panel (hidden until the button is clicked).
  const panel = document.createElement('div')
  panel.className = 'bythos-panel'
  panel.hidden = true
  panel.innerHTML = `
    <p class="bythos-title">Guardar en Bythos</p>
    <label class="bythos-label" for="bythos-folder">Carpeta</label>
    <select class="bythos-select" id="bythos-folder"></select>
    <p class="bythos-msg"></p>
    <div class="bythos-actions">
      <button class="bythos-send">Enviar</button>
      <button class="bythos-cancel">Cancelar</button>
    </div>
  `
  shadow.appendChild(panel)

  const select = panel.querySelector('.bythos-select')
  const msg = panel.querySelector('.bythos-msg')
  const sendBtn = panel.querySelector('.bythos-send')
  const cancelBtn = panel.querySelector('.bythos-cancel')

  // Read the favorite folder (shared key with popup.js).
  async function leerFavorita() {
    try {
      const saved = await chrome.storage.local.get(['folder'])
      return saved.folder ? String(saved.folder) : ''
    } catch {
      return ''
    }
  }

  // Load folder names each time the panel opens (list may have changed).
  async function cargarCarpetas() {
    msg.textContent = 'Cargando carpetas…'
    sendBtn.disabled = true
    select.innerHTML = ''
    try {
      const res = await fetch(`${API}/api/carpetas`)
      const data = await res.json()
      const carpetas = (Array.isArray(data) ? data : []).map(normCarpeta)
      if (carpetas.length === 0) {
        msg.textContent = 'Crea primero una carpeta en la app.'
        return
      }
      for (const c of carpetas) {
        const opt = document.createElement('option')
        opt.value = String(c.id)
        opt.textContent = c.nombre
        select.appendChild(opt)
      }
      const fav = await leerFavorita()
      if (fav && carpetas.some((c) => String(c.id) === fav)) select.value = fav
      msg.textContent = ''
      sendBtn.disabled = false
    } catch {
      // fetch only fails like this when the desktop app is off.
      msg.textContent = 'Abre primero tu app Bythos (.exe en :8080).'
    }
  }

  fab.addEventListener('click', () => {
    panel.hidden = !panel.hidden
    if (!panel.hidden) cargarCarpetas()
  })

  cancelBtn.addEventListener('click', () => {
    panel.hidden = true
  })

  document.addEventListener('keydown', (e) => {
    if (e.key === 'Escape') panel.hidden = true
  })

  sendBtn.addEventListener('click', async () => {
    const carpeta_id = Number(select.value)
    if (!carpeta_id) {
      msg.textContent = 'Elige una carpeta para guardar.'
      return
    }
    // Remember preference for next time (same key as popup.js).
    try {
      chrome.storage.local.set({ folder: carpeta_id })
    } catch {
      // storage is a convenience, never a blocker.
    }
    msg.textContent = 'Guardando…'
    try {
      // Same contract as the desktop UI: {carpeta_id, url}. Born pendiente in Go.
      const res = await fetch(`${API}/api/recursos`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ carpeta_id, url: location.href }),
      })
      const data = await res.json().catch(() => ({}))
      msg.textContent = res.ok
        ? 'Guardado en tu PC.'
        : `${data.error || 'Carpeta inexistente (créala en la app).'}`
    } catch {
      msg.textContent = 'Abre primero tu app Bythos (.exe en :8080).'
    }
  })
})()
