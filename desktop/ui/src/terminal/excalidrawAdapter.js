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

// Lo que viaja al servidor: sin imágenes ni borrados; el fondo es lo único del appState.
export function sanearEscena(elements, appState) {
  return {
    elements: elements.filter((e) => !e.isDeleted && e.type !== 'image'),
    appState: { viewBackgroundColor: appState?.viewBackgroundColor ?? '#ffffff' },
    files: {},
  }
}
