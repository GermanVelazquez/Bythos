import { useEffect, useRef } from 'react'
import { api } from '../api.js'
import { actualizarEscena, exportarPng } from './excalidrawAdapter.js'
import { aplicarOps, camposDeshacer } from './opsPizarra.js'

const LATIDO_MS = 1000

// Puente entre el agente (MCP) y el lienzo: mientras la pizarra está montada manda un
// latido por segundo, aplica los lotes de ops que vuelven y responde las capturas.
// Un latido en vuelo a la vez. Un lote ya aplicado nunca se reaplica (mover no es
// idempotente): si su ack se perdió, solo se repite el ack.
export function usePizarraAgente(sesionId, excalidrawApi, vaciar) {
  const vaciarRef = useRef(vaciar)
  vaciarRef.current = vaciar

  useEffect(() => {
    if (!excalidrawApi) return undefined
    let vivo = true
    let enVuelo = false
    const resultados = new Map() // id de op -> cuerpo del ack ya calculado
    const capturadas = new Set()

    async function aplicar(o) {
      let cuerpo = resultados.get(o.id)
      if (!cuerpo) {
        try {
          const ops = typeof o.op === 'string' ? JSON.parse(o.op) : o.op
          if (!Array.isArray(ops)) throw new Error('Lote de operaciones inválido')
          const antes = excalidrawApi.getSceneElementsIncludingDeleted()
          const { elementos, ids } = aplicarOps(antes, ops)
          // La inversa se calcula antes de aplicar y viaja en el ack (D13): sin ella el lote no se deshace.
          cuerpo = { ok: true, ids, ...camposDeshacer(antes, elementos) }
          actualizarEscena(excalidrawApi, elementos)
        } catch (e) {
          cuerpo = { ok: false, ids: [], error: e.message }
        }
        resultados.set(o.id, cuerpo)
      }
      await api.ackOpPizarra(o.id, cuerpo).catch(() => {}) // el próximo latido reintenta el ack
    }

    async function capturar(id) {
      if (capturadas.has(id)) return
      capturadas.add(id)
      try {
        await vaciarRef.current() // lo guardado y lo capturado coinciden
        const png = await exportarPng(
          excalidrawApi.getSceneElements(),
          { ...excalidrawApi.getAppState(), exportBackground: true },
          excalidrawApi.getFiles(),
        )
        await api.subirCapturaPizarra(id, png)
      } catch { /* sin PNG el servidor da su aviso al agente por tiempo agotado */ }
    }

    async function latir() {
      if (enVuelo) return
      enVuelo = true
      try {
        const r = await api.latidoPizarra(sesionId)
        for (const o of r?.ops ?? []) if (vivo) await aplicar(o)
        for (const id of r?.capturas ?? []) if (vivo) await capturar(id)
      } catch { /* app sin respuesta: se reintenta en el próximo latido */ } finally {
        enVuelo = false
      }
    }

    latir()
    const t = setInterval(latir, LATIDO_MS)
    return () => { vivo = false; clearInterval(t) }
  }, [sesionId, excalidrawApi])
}
