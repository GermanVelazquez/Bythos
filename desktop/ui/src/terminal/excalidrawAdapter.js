// ÚNICO importador de @excalidraw/excalidraw (0.18.1, fijado sin ^). Todo el
// resto de la UI habla con esta capa; si Excalidraw cambia, se arregla aquí.
import {
  Excalidraw, convertToExcalidrawElements, exportToBlob, getSceneVersion, CaptureUpdateAction,
} from '@excalidraw/excalidraw'
import '@excalidraw/excalidraw/index.css'

export { Excalidraw }

// regenerateIds:false conserva los ids dados (comprobado en 0.18.1: el código
// solo asigna id nuevo cuando regenerateIds !== false).
export const convertirElementos = (esqueleto) =>
  convertToExcalidrawElements(esqueleto, { regenerateIds: false })

// captureUpdate IMMEDIATELY: el cambio entra al historial y el usuario puede Ctrl+Z.
export const actualizarEscena = (api, elements, captureUpdate = CaptureUpdateAction.IMMEDIATELY) =>
  api.updateScene({ elements, captureUpdate })

export const exportarPng = (elements, appState, files, maxWidthOrHeight = 1280) =>
  exportToBlob({ elements, appState, files, mimeType: 'image/png', maxWidthOrHeight })

export const versionEscena = (elements) => getSceneVersion(elements)

const TIPOS_PROHIBIDOS = new Set(['embeddable', 'iframe', 'magicframe'])
const MAX_LADO_IMAGEN = 800 // lado mayor al insertar; el archivo conserva su tamaño real
const NOMBRE_TARJETA = 28

// sha referido por un elemento vivo (imagen: fileId; tarjeta: customData.sha256).
export const shaDeElemento = (e) => (e.type === 'image' ? e.fileId : e.customData?.sha256) ?? null

// Lo que viaja al servidor: sin borrados ni bytes (files:{}); el fondo es lo único del appState.
// Imágenes y tarjetas solo si su sha es un adjunto conocido de la sesión (D9); `conocidos`
// null = no filtrar (fallo al listar: el servidor decide y nunca se pierde nada en silencio).
export function sanearEscena(elements, appState, conocidos = null) {
  const desconocido = (e) => {
    const sha = shaDeElemento(e)
    return sha == null ? false : !conocidos || !conocidos.has(sha)
  }
  const quitados = new Set()
  const vivos = elements.filter((e) => {
    const fuera = e.isDeleted || TIPOS_PROHIBIDOS.has(e.type) || (conocidos && desconocido(e))
    if (fuera) quitados.add(e.id)
    return !fuera
  })
  return {
    elements: vivos.filter((e) => !(e.containerId && quitados.has(e.containerId))),
    appState: { viewBackgroundColor: appState?.viewBackgroundColor ?? '#ffffff' },
    files: {},
  }
}

// Imagen del adjunto: fileId = sha256, ya guardada en el servidor (status "saved").
export const construirImagen = (adjunto, x, y, ancho, alto) => {
  const f = Math.min(1, MAX_LADO_IMAGEN / Math.max(ancho, alto, 1))
  return {
    type: 'image', id: crypto.randomUUID(), x, y, fileId: adjunto.sha256, status: 'saved',
    width: Math.round(ancho * f), height: Math.round(alto * f),
  }
}

// Tarjeta de un documento: rectángulo con texto ligado (nombre y tipo). Va como
// esqueleto: pásalo por convertirElementos. Sin `link`: abrir la tarjeta es de la PR 5b.
export function construirTarjeta(adjunto, x, y) {
  const nombre = adjunto.nombre.length > NOMBRE_TARJETA ? `${adjunto.nombre.slice(0, NOMBRE_TARJETA - 1)}…` : adjunto.nombre
  return {
    type: 'rectangle', id: crypto.randomUUID(), x, y, width: 280, height: 90,
    backgroundColor: '#2a2a2a', fillStyle: 'solid', roundness: { type: 3 },
    customData: { bythos: 'adjunto', sha256: adjunto.sha256 },
    label: { text: `${nombre}
${(adjunto.nombre.includes('.') ? adjunto.nombre.split('.').pop() : adjunto.tipo).toUpperCase()}`, fontSize: 18 },
  }
}

// D8: primero los bytes (addFiles), luego los elementos con captureUpdate IMMEDIATELY.
// `dataURL` son los bytes realmente subidos (ya reducidos); `ancho`/`alto` 0 si no se midieron.
export function insertarAdjunto(api, adjunto, dataURL, ancho, alto) {
  const { scrollX, scrollY, zoom, width, height } = api.getAppState()
  const cx = -scrollX + width / 2 / zoom.value
  const cy = -scrollY + height / 2 / zoom.value
  let nuevos
  if (adjunto.tipo === 'imagen') {
    api.addFiles([{ id: adjunto.sha256, mimeType: adjunto.mime, dataURL, created: Date.now() }])
    const img = construirImagen(adjunto, 0, 0, ancho || 300, alto || 300)
    nuevos = convertirElementos([{ ...img, x: cx - img.width / 2, y: cy - img.height / 2 }])
  } else {
    nuevos = convertirElementos([construirTarjeta(adjunto, cx - 140, cy - 45)])
  }
  actualizarEscena(api, [...api.getSceneElementsIncludingDeleted(), ...nuevos])
}
