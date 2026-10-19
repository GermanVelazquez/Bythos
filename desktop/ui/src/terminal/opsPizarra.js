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

const ALTO_LINEA = 1.25 // lineHeight de Excalidraw: alto de una línea = fontSize * 1.25

// Ajustes por tipo antes de convertir: el texto toma su tamaño de letra del alto
// (si no, Excalidraw lo deja en 20 y ancho/alto solo estiran la caja), y las
// líneas y trazos miden su caja a partir de los puntos.
function ajustar(s) {
  if (s.type === 'text') {
    if (s.height) s.fontSize = Math.max(8, Math.round(s.height / ALTO_LINEA))
    delete s.width
    delete s.height
  }
  if (Array.isArray(s.points) && s.points.length) {
    const xs = s.points.map((p) => p[0]), ys = s.points.map((p) => p[1])
    s.width = Math.max(...xs) - Math.min(...xs)
    s.height = Math.max(...ys) - Math.min(...ys)
  }
  return s
}

// El conversor no arma freedraw (lo pasa tal cual, sin presiones y no se dibuja):
// se convierte como línea y se completa como trazo a mano.
const comoTrazo = (e) => {
  const { startBinding, endBinding, startArrowhead, endArrowhead, polygon, ...resto } = e
  return { ...resto, type: 'freedraw', points: densificar(e.points), pressures: [], simulatePressure: true, lastCommittedPoint: null }
}

// El trazo a mano suaviza mucho los puntos ralos (un zigzag sale como curva):
// se intercala un punto cada ~4 px, como los que deja el mouse (tope ~4000 puntos).
function densificar(puntos) {
  const out = puntos.length ? [puntos[0]] : []
  let largo = 0
  for (let i = 1; i < puntos.length; i++) largo += Math.hypot(puntos[i][0] - puntos[i - 1][0], puntos[i][1] - puntos[i - 1][1])
  const paso = Math.max(4, largo / 4000)
  for (let i = 1; i < puntos.length; i++) {
    const [ax, ay] = puntos[i - 1], [bx, by] = puntos[i]
    const pasos = Math.max(1, Math.ceil(Math.hypot(bx - ax, by - ay) / paso))
    for (let k = 1; k <= pasos; k++) out.push([ax + ((bx - ax) * k) / pasos, ay + ((by - ay) * k) / pasos])
  }
  return out
}

function convertir(esqueletos) {
  const trazos = new Set(esqueletos.filter((s) => s.type === 'freedraw').map((s) => s.id))
  const lista = esqueletos.map((s) => ajustar(trazos.has(s.id) ? { ...s, type: 'line' } : s))
  return convertirElementos(lista).map((e) => (trazos.has(e.id) ? comoTrazo(e) : e))
}

// Marca del agente: por customData o, si Excalidraw lo perdió (R2), por el prefijo del id.
export const esMarca = (e) => e?.customData?.bythos === 'marca' || String(e?.id ?? '').startsWith('marca-')

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
    const { tipo, desde, hasta, datos, ...resto } = o.elemento
    const s = { type: tipo, id: o.elemento.id, ...propiedades(resto) }
    if (datos) s.customData = datos // marca del agente: el servidor la etiqueta (D3)
    if (tipo !== 'text' && s.text) { s.label = { text: s.text }; delete s.text; delete s.originalText } // texto dentro de la forma
    if (desde) s.start = { id: desde }
    if (hasta) s.end = { id: hasta }
    return s
  })
  const convertidos = convertir(esqueletos)
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
        for (const nuevoEl of [el, ...textosDe(convertidos, el.id)]) {
          const previo = mapa.get(nuevoEl.id)
          // Una marca reemplazada sube de versión: sin eso Excalidraw la reconcilia como igual (D5).
          const e = previo && esMarca(previo) ? { ...nuevoEl, version: (previo.version ?? 0) + 1 } : nuevoEl
          mapa.delete(e.id) // reubica al final si ya existía
          mapa.set(e.id, e)
        }
        tocados.add(el.id)
        break
      }
      case 'actualizar': {
        const e = vivo(op.id)
        let cambios = propiedades(op.cambios)
        if (e.type === 'text' && (cambios.text !== undefined || cambios.height !== undefined)) {
          // Se vuelve a medir el texto con el nuevo contenido o tamaño de letra.
          const [medido] = convertir([{ type: 'text', id: e.id, x: e.x, y: e.y, text: e.text, fontSize: e.fontSize,
            fontFamily: e.fontFamily, strokeColor: e.strokeColor, ...cambios }])
          cambios = { ...cambios, text: medido.text, originalText: medido.originalText, fontSize: medido.fontSize,
            width: medido.width, height: medido.height }
        } else if (cambios.points) {
          cambios = ajustar({ ...cambios })
        }
        mapa.set(op.id, nueva(e, cambios))
        tocados.add(op.id)
        break
      }
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
