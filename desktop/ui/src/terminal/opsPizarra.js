import { convertirElementos } from './excalidrawAdapter.js'

// Traduce los campos en español de pizarra_dibujar a propiedades de Excalidraw.
const CAMPOS = {
  x: 'x', y: 'y', ancho: 'width', alto: 'height', texto: 'text', color: 'strokeColor',
  fondo: 'backgroundColor', grosor: 'strokeWidth', puntos: 'points',
}

function propiedades(e) {
  const p = {}
  for (const [es, en] of Object.entries(CAMPOS)) if (e[es] !== undefined) p[en] = e[es]
  if (p.text !== undefined) p.originalText = p.text
  return p
}

const nueva = (el, cambios) => ({
  ...el, ...cambios, version: (el.version ?? 0) + 1, versionNonce: Math.floor(Math.random() * 2 ** 31), updated: Date.now(),
})

// Aplica un lote de ops sobre la lista de elementos (incluye borrados). ATÓMICO:
// si algún id objetivo no existe lanza y no devuelve nada. Mezcla por id, así
// reaplicar un "agregar" no duplica. Devuelve { elementos, ids } (ids tocados).
export function aplicarOps(elementos, ops) {
  const mapa = new Map(elementos.map((e) => [e.id, e]))
  const tocados = new Set()

  // Los "agregar" se convierten juntos: así una flecha puede enlazar (desde/hasta) a un
  // elemento agregado en el mismo lote.
  const esqueletos = ops.filter((o) => o.accion === 'agregar').map((o) => {
    const { tipo, desde, hasta, ...resto } = o.elemento
    const s = { type: tipo, id: o.elemento.id, ...propiedades(resto) }
    if (tipo !== 'text' && s.text) { s.label = { text: s.text }; delete s.text; delete s.originalText } // texto dentro de la forma
    if (desde) s.start = { id: desde }
    if (hasta) s.end = { id: hasta }
    return s
  })
  const convertidos = convertirElementos(esqueletos)
  const agregados = new Map(convertidos.map((e) => [e.id, e]))
  // El texto de una forma (label) es otro elemento con containerId: viaja con ella.
  const textosDe = (lista, id) => lista.filter((e) => e.containerId === id && !e.isDeleted)

  const vivo = (id) => {
    const e = mapa.get(id)
    if (!e || e.isDeleted) throw new Error(`No existe el elemento "${id}"`)
    return e
  }

  for (const op of ops) {
    switch (op.accion) {
      case 'agregar': {
        const el = agregados.get(op.elemento.id)
        for (const e of [el, ...textosDe(convertidos, el.id)]) {
          mapa.delete(e.id) // reubica al final si ya existía
          mapa.set(e.id, e)
        }
        tocados.add(el.id)
        break
      }
      case 'actualizar':
        mapa.set(op.id, nueva(vivo(op.id), propiedades(op.cambios)))
        tocados.add(op.id)
        break
      case 'borrar':
        op.ids.forEach((id) => {
          for (const e of [vivo(id), ...textosDe([...mapa.values()], id)]) mapa.set(e.id, nueva(e, { isDeleted: true }))
          tocados.add(id)
        })
        break
      case 'mover':
        op.ids.forEach((id) => {
          for (const e of [vivo(id), ...textosDe([...mapa.values()], id)]) mapa.set(e.id, nueva(e, { x: e.x + op.dx, y: e.y + op.dy }))
          tocados.add(id)
        })
        break
      default:
        throw new Error(`Acción desconocida "${op.accion}"`)
    }
  }
  return { elementos: [...mapa.values()], ids: [...tocados] }
}
