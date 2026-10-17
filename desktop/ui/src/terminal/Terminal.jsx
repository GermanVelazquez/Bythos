import { useEffect, useState } from 'react'
import { api, TEXTO_TERMINAL } from '../api.js'
import PestanasSesiones, { MODOS } from './PestanasSesiones.jsx'
import DialogoNuevaSesion from './DialogoNuevaSesion.jsx'
import PanelTerminal from './PanelTerminal.jsx'

const CLAVE_ABIERTAS = 'bythos.terminal.abiertas'
const CLAVE_ACTIVA = 'bythos.terminal.activa'

function leerLocal(clave, defecto) {
  try { return JSON.parse(localStorage.getItem(clave)) ?? defecto } catch { return defecto }
}

// Contenedor: sesiones, pestañas abiertas y activa (localStorage). Cerrar una
// pestaña solo la quita de localStorage; borrar pide confirmación y mata el PTY.
export default function Terminal({ carpetas }) {
  const [sesiones, setSesiones] = useState([])
  const [abiertas, setAbiertas] = useState(() => leerLocal(CLAVE_ABIERTAS, []))
  const [activa, setActiva] = useState(() => leerLocal(CLAVE_ACTIVA, null))
  const [dialogo, setDialogo] = useState(false)
  const [errorDialogo, setErrorDialogo] = useState('')
  const [error, setError] = useState('')

  useEffect(() => { localStorage.setItem(CLAVE_ABIERTAS, JSON.stringify(abiertas)) }, [abiertas])
  useEffect(() => { localStorage.setItem(CLAVE_ACTIVA, JSON.stringify(activa)) }, [activa])

  useEffect(() => {
    api.listarSesiones()
      .then((lista) => {
        const l = Array.isArray(lista) ? lista : []
        setSesiones(l)
        // Descarta pestañas de sesiones que ya no existen.
        setAbiertas((a) => a.filter((id) => l.some((s) => s.id === id)))
      })
      .catch((e) => setError(e.message))
  }, [])

  const nombreCarpeta = (s) => carpetas.find((c) => c.id === s.carpeta_id)?.nombre ?? ''
  const porId = (id) => sesiones.find((s) => s.id === id)
  const pestanas = abiertas.map(porId).filter(Boolean)
  const sesionActiva = pestanas.find((s) => s.id === activa) ?? null

  function abrir(id) {
    setAbiertas((a) => (a.includes(id) ? a : [...a, id]))
    setActiva(id)
  }

  function cerrarPestana(id) {
    setAbiertas((a) => a.filter((x) => x !== id))
    if (activa === id) setActiva(abiertas.find((x) => x !== id) ?? null)
  }

  async function crear(nombre, modo, carpetaId) {
    try {
      const s = await api.crearSesion(nombre, modo, carpetaId)
      if (!Number.isInteger(s?.id)) throw new Error('No se pudo crear la sesión')
      setSesiones((l) => [s, ...l])
      setDialogo(false)
      setErrorDialogo('')
      abrir(s.id)
    } catch (e) {
      setErrorDialogo(e.message)
    }
  }

  async function borrar(s) {
    if (!window.confirm(`¿Eliminar la sesión "${s.nombre}"? Termina su terminal y borra su pizarra.`)) return
    cerrarPestana(s.id) // desmonta el panel (cierra el WS) antes de borrar
    try {
      await api.borrarSesion(s.id)
      setSesiones((l) => l.filter((x) => x.id !== s.id))
    } catch (e) {
      setError(e.message)
    }
  }

  return (
    <div className="term-vista">
      <PestanasSesiones
        sesiones={pestanas} activa={activa} nombreCarpeta={nombreCarpeta}
        onElegir={setActiva} onCerrar={cerrarPestana} onNueva={() => setDialogo(true)}
      />
      {error && <div className="error-banner"><span>{error}</span></div>}
      <div className="term-cuerpo">
        <aside className="term-lista">
          {sesiones.map((s) => (
            <div key={s.id} className={`term-item${s.id === activa ? ' active' : ''}`}>
              <button className="term-item-abrir" onClick={() => abrir(s.id)}>
                <span>{s.nombre}</span>
                <small>{MODOS[s.modo] ?? s.modo} · {nombreCarpeta(s) || TEXTO_TERMINAL.suelta}</small>
              </button>
              <button className="term-tab-x" title="Eliminar sesión" onClick={() => borrar(s)}>🗑</button>
            </div>
          ))}
        </aside>
        <section className="term-area">
          {sesionActiva
            ? <PanelTerminal key={sesionActiva.id} sesion={sesionActiva} />
            : <div className="empty"><p className="empty-title">{TEXTO_TERMINAL.nav}</p><p className="empty-sub">Crea o abre una sesión.</p></div>}
        </section>
      </div>
      {dialogo && (
        <DialogoNuevaSesion
          carpetas={carpetas} error={errorDialogo}
          onCrear={crear} onCerrar={() => { setDialogo(false); setErrorDialogo('') }}
        />
      )}
    </div>
  )
}
