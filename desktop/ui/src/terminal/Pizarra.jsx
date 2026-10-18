import { useRef, useState } from 'react'
import { Excalidraw, sanearEscena, versionEscena } from './excalidrawAdapter.js'
import { TEXTO_PIZARRA } from '../api.js'

const UI_OPCIONES = {
  tools: { image: false },
  canvasActions: { loadScene: false, saveToActiveFile: false, export: false, saveAsImage: false },
}

const traeArchivos = (dt) => Array.from(dt?.types ?? []).includes('Files')

// Presentacional: lienzo de Excalidraw. Reporta (escena saneada, clave de versión)
// y no sabe nada de guardado. Sin imágenes: herramienta oculta, pegar y soltar
// bloqueados en captura (antes de que Excalidraw los vea).
export default function Pizarra({ inicial, alCambiar }) {
  const [aviso, setAviso] = useState('')
  const ultima = useRef(null)

  function bloquear(e) {
    if (!traeArchivos(e.clipboardData ?? e.dataTransfer)) return
    e.preventDefault()
    e.stopPropagation()
    setAviso(TEXTO_PIZARRA.sinImagenes)
    clearTimeout(ultima.current)
    ultima.current = setTimeout(() => setAviso(''), 4000)
  }

  // Segunda barrera: si algún pegado llegara con archivos o imágenes, no se aplica.
  const alPegar = (datos) =>
    !(Object.keys(datos?.files ?? {}).length || (datos?.elements ?? []).some((e) => e.type === 'image'))

  return (
    <div className="term-lienzo" onDropCapture={bloquear} onPasteCapture={bloquear}>
      <Excalidraw
        initialData={{ elements: inicial.elements, appState: inicial.appState, scrollToContent: true }}
        UIOptions={UI_OPCIONES}
        aiEnabled={false}
        theme="dark"
        onPaste={alPegar}
        langCode="es-ES"
        onChange={(elements, appState) => alCambiar(sanearEscena(elements, appState), `${versionEscena(elements)}|${appState.viewBackgroundColor}`)}
      />
      {aviso && <div className="term-aviso">{aviso}</div>}
    </div>
  )
}
