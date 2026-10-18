import { useState } from 'react'
import { TEXTO_TERMINAL } from '../api.js'
import { MODOS } from './PestanasSesiones.jsx'

// Presentacional: nombre (obligatorio), carpeta (opcional) y modo.
export default function DialogoNuevaSesion({ carpetas, error, onCrear, onCerrar }) {
  const [nombre, setNombre] = useState('')
  const [carpetaId, setCarpetaId] = useState('')
  const [modo, setModo] = useState('practica')
  const [vacio, setVacio] = useState(false)

  function enviar(e) {
    e.preventDefault()
    if (!nombre.trim()) return setVacio(true)
    onCrear(nombre.trim(), modo, carpetaId ? Number(carpetaId) : null)
  }

  return (
    <div className="modal-overlay animate-fade-in" onClick={onCerrar}>
      <form className="modal animate-slide-up" onClick={(e) => e.stopPropagation()} onSubmit={enviar}>
        <div className="modal-head">
          <h2 className="modal-title">Nueva sesión</h2>
          <button type="button" className="modal-close" onClick={onCerrar}>×</button>
        </div>
        <div className="field">
          <label className="field-label">Nombre</label>
          <input autoFocus value={nombre} onChange={(e) => { setNombre(e.target.value); setVacio(false) }} />
          {vacio && <p className="field-error">{TEXTO_TERMINAL.nombreVacio}</p>}
        </div>
        <div className="field">
          <label className="field-label">Carpeta (opcional)</label>
          <select value={carpetaId} onChange={(e) => setCarpetaId(e.target.value)}>
            <option value="">{TEXTO_TERMINAL.suelta}</option>
            {carpetas.map((c) => <option key={c.id} value={c.id}>{c.nombre}</option>)}
          </select>
        </div>
        <div className="field">
          <label className="field-label">Modo</label>
          <select value={modo} onChange={(e) => setModo(e.target.value)}>
            {Object.entries(MODOS).map(([k, v]) => <option key={k} value={k}>{v}</option>)}
          </select>
        </div>
        {error && <p className="field-error">{error}</p>}
        <div className="modal-actions">
          <button type="button" className="btn-outline" onClick={onCerrar}>Cancelar</button>
          <button type="submit" className="btn-primary">Crear</button>
        </div>
      </form>
    </div>
  )
}
