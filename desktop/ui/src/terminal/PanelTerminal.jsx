import { useEffect, useRef, useState } from 'react'
import { Terminal as Xterm } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import '@xterm/xterm/css/xterm.css'
import { api, TEXTO_TERMINAL } from '../api.js'
import { conectarPty, sinControl } from './conexionPty.js'
import { ChipTerminal } from './ChipsEstado.jsx'

const TEXTO_ESTADO = {
  conectando: 'Conectando…',
  conectado: 'Conectado',
  reconectando: 'Reconectando…',
  terminado: 'Proceso terminado',
  reemplazada: 'Abierta en otra ventana',
  sinconpty: TEXTO_TERMINAL.sinConPty,
  limite: 'Hay demasiadas terminales abiertas (máximo 8)',
}

// Presentacional: monta xterm y lo conecta a la sesión. Sin estado de negocio.
export default function PanelTerminal({ sesion, enfocarRef, escribirRef }) {
  const contenedor = useRef(null)
  const [estado, setEstado] = useState({ clave: 'conectando' })
  const [agente, setAgente] = useState('')

  useEffect(() => {
    const term = new Xterm({
      fontFamily: 'Consolas, "Cascadia Mono", monospace',
      fontSize: 13,
      cursorBlink: true,
      theme: { background: '#0a0a0a', foreground: '#ededed' },
    })
    const fit = new FitAddon()
    term.loadAddon(fit)
    term.open(contenedor.current)

    const conexion = conectarPty(sesion.id, {
      alDatos: (d) => term.write(d),
      alReiniciar: () => term.reset(),
      alEstado: (clave, extra) => setEstado({ clave, extra }),
    })
    const ajustar = () => {
      try { fit.fit() } catch { return }
      conexion.redimensionar(term.cols, term.rows)
    }
    ajustar()
    term.focus()
    if (enfocarRef) enfocarRef.current = () => term.focus()
    // Pega (sin Enter) una sola línea; false y nada escrito si no hay conexión.
    if (escribirRef) {
      escribirRef.current = (texto) => {
        if (!conexion.abierta()) return false
        const linea = sinControl(texto)
        if (linea) term.paste(linea)
        return true
      }
    }

    const datos = term.onData((t) => conexion.enviar(t))

    // Portapapeles: Ctrl+V no hace preventDefault para que el paste nativo
    // llegue una sola vez a xterm; Ctrl+R pasa al shell pero no recarga.
    term.attachCustomKeyEventHandler((e) => {
      if (e.type !== 'keydown' || !e.ctrlKey) return true
      const k = e.key.toLowerCase()
      if (k === 'c' && (e.shiftKey || term.hasSelection())) {
        if (term.hasSelection()) navigator.clipboard?.writeText(term.getSelection())
        return false
      }
      if (k === 'v') return false
      if (k === 'r') { e.preventDefault(); return true }
      return true
    })

    // Recargar la ventana mataría la vista, no el proceso, pero molesta: se bloquea.
    const bloquear = (e) => {
      if (e.key === 'F5' || (e.ctrlKey && e.key.toLowerCase() === 'r')) e.preventDefault()
    }
    window.addEventListener('keydown', bloquear, true)

    let temporizador = null
    const observador = new ResizeObserver(() => {
      clearTimeout(temporizador)
      temporizador = setTimeout(ajustar, 100)
    })
    observador.observe(contenedor.current)

    return () => {
      window.removeEventListener('keydown', bloquear, true)
      if (enfocarRef) enfocarRef.current = null
      if (escribirRef) escribirRef.current = null
      clearTimeout(temporizador)
      observador.disconnect()
      datos.dispose()
      conexion.cerrar()
      term.dispose()
    }
  }, [sesion.id])

  async function abrirAgente() {
    try {
      const r = await api.abrirTerminalAgente()
      setAgente(`Terminal abierta (${r?.terminal ?? r?.Terminal ?? 'terminal'}).`)
    } catch (e) {
      setAgente(e.message || 'No se pudo abrir la terminal')
    }
  }

  const texto = TEXTO_ESTADO[estado.clave] + (estado.clave === 'terminado' ? ` (código ${estado.extra})` : '')
  const sinConPty = estado.clave === 'sinconpty'
  return (
    <div className="term-panel">
      <div className="term-estado">
        <ChipTerminal clave={estado.clave} texto={texto} />
        {sinConPty && <button className="btn-outline" onClick={abrirAgente}>Abrir agente</button>}
        {agente && <span className="field-hint">{agente}</span>}
      </div>
      <div className="term-xterm" ref={contenedor} />
    </div>
  )
}
