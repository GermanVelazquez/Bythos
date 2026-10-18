import { useCallback, useEffect, useRef, useState } from 'react'
import { api, TEXTO_PIZARRA } from '../api.js'
import { cargarArchivos } from './adjuntos.js'

const DEBOUNCE_MS = 1500
const LIMITE_KEEPALIVE = 60 * 1024 // tope de fetch keepalive del navegador (64 KiB)

// Autoguardado de la pizarra de una sesión (design D10/D11): la escena vive en
// la UI, se guarda con debounce y se vacía al cambiar de pestaña, desmontar,
// ocultar o cerrar la página. Un guardado fallido deja la escena en memoria.
export function useAutoguardado(sesionId) {
  const [carga, setCarga] = useState(null) // { escena, files, conocidos, n }: n fuerza remontar el lienzo
  const [guardado, setGuardado] = useState('guardado')
  const [aviso, setAviso] = useState('')
  const r = useRef({ version: 0, clave: null, pendiente: null, timer: null, cola: Promise.resolve(), cache: new Map() })

  const cargar = useCallback(async () => {
    const d = await api.leerPizarra(sesionId)
    if (typeof d?.version !== 'number' || !Array.isArray(d?.escena?.elements)) {
      throw new Error('Respuesta de pizarra inválida')
    }
    // Adjuntos y bytes antes de montar el lienzo (excalidraw#6146). Sin lista, `conocidos`
    // es null (sanearEscena no filtra) y el tablero carga sin bytes.
    let conocidos = null
    let files = {}
    let faltan = 0
    try {
      const lista = await api.listarAdjuntos(sesionId)
      conocidos = new Set(lista.map((a) => a.sha256))
      ;({ files, faltan } = await cargarArchivos(sesionId, lista, d.escena.elements, r.current.cache))
    } catch {
      setAviso(TEXTO_PIZARRA.adjuntosSinListar)
    }
    if (faltan) setAviso(TEXTO_PIZARRA.adjuntoNoDisponible)
    Object.assign(r.current, { version: d.version, clave: null, pendiente: null })
    setCarga((c) => ({ escena: d.escena, files, conocidos, n: (c?.n ?? 0) + 1 }))
  }, [sesionId])

  const enviar = useCallback(async (keepalive) => {
    const e = r.current
    const p = e.pendiente
    if (!p) return
    e.pendiente = null
    setGuardado('guardando')
    try {
      const ka = keepalive && JSON.stringify(p).length < LIMITE_KEEPALIVE
      const res = await api.guardarPizarra(sesionId, p, e.version, ka)
      if (res.estado === 409) {
        await cargar()
        setAviso(TEXTO_PIZARRA.otraVentana)
        setGuardado('guardado')
        return
      }
      if (!res.ok || !Number.isInteger(res.version)) throw new Error(res.error || 'No se pudo guardar')
      e.version = res.version
      if (!e.pendiente) setGuardado('guardado')
    } catch {
      e.pendiente ??= p // se reintenta en el próximo cambio o vaciado
      setGuardado('error')
    }
  }, [sesionId, cargar])

  const vaciar = useCallback((keepalive = false) => {
    clearTimeout(r.current.timer)
    r.current.cola = r.current.cola.then(() => enviar(keepalive))
    return r.current.cola
  }, [enviar])

  const alCambiar = useCallback((escena, clave) => {
    const e = r.current
    if (e.clave === null) { e.clave = clave; return } // primer onChange = escena cargada
    if (clave === e.clave) return // solo cambió la vista (zoom, scroll)
    e.clave = clave
    e.pendiente = escena
    setGuardado('guardando')
    clearTimeout(e.timer)
    e.timer = setTimeout(() => vaciar(), DEBOUNCE_MS)
  }, [vaciar])

  useEffect(() => {
    cargar().catch((err) => { setAviso(err.message); setGuardado('error') })
    const oculta = () => { if (document.visibilityState === 'hidden') vaciar() }
    const cierra = () => vaciar(true)
    document.addEventListener('visibilitychange', oculta)
    window.addEventListener('pagehide', cierra)
    return () => {
      document.removeEventListener('visibilitychange', oculta)
      window.removeEventListener('pagehide', cierra)
      vaciar() // desmontar (cambio de pestaña o de pantalla)
    }
  }, [cargar, vaciar])

  return { carga, guardado, aviso, alCambiar, vaciar }
}
