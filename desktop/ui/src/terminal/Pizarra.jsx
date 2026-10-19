import { useEffect, useRef, useState } from 'react'
import { Excalidraw, sanearEscena, versionEscena, insertarAdjunto, shaDeElemento } from './excalidrawAdapter.js'
import { subirArchivo, leerComoDataURL } from './adjuntos.js'
import { TEXTO_PIZARRA } from '../api.js'

const UI_OPCIONES = {
  tools: { image: false },
  canvasActions: { loadScene: false, saveToActiveFile: false, export: false, saveAsImage: false },
}

const traeArchivos = (dt) => Array.from(dt?.types ?? []).includes('Files')

// Archivos de un pegado: files y, si viene vacío, items (getAsFile).
const archivosDe = (dt) => {
  const directos = Array.from(dt?.files ?? [])
  if (directos.length) return directos
  return Array.from(dt?.items ?? []).filter((i) => i.kind === 'file').map((i) => i.getAsFile()).filter(Boolean)
}

// Presentacional: lienzo de Excalidraw. Reporta (escena saneada, clave de versión).
// Pegar y soltar se interceptan en captura (antes de que Excalidraw los vea) y van
// al flujo de adjuntos: subir -> addFiles -> updateScene -> vaciar. Las imágenes
// solo entran por ahí (herramienta oculta).
// `archivos` y `conocidosIniciales` los trae useAutoguardado.cargar(): los bytes van en
// initialData.files y los conocidos (null = no se pudo listar) ya existen al montar.
export default function Pizarra({ sesionId, inicial, archivos, conocidosIniciales, alCambiar, alTenerApi, vaciar }) {
  const [aviso, setAviso] = useState('')
  const ultima = useRef(null)
  const apiLienzo = useRef(null)
  const selector = useRef(null)
  const conocidos = useRef(conocidosIniciales)

  useEffect(() => () => clearTimeout(ultima.current), [])

  function mostrar(texto) {
    setAviso(texto)
    clearTimeout(ultima.current)
    ultima.current = setTimeout(() => setAviso(''), 4000)
  }

  async function adjuntar(archivos) {
    for (const original of archivos) {
      try {
        const { adjunto, archivo, ancho, alto } = await subirArchivo(sesionId, original)
        const dataURL = adjunto.tipo === 'imagen' ? await leerComoDataURL(archivo) : null
        conocidos.current?.add(adjunto.sha256) // antes de pintar: el onChange ya debe conocerlo
        insertarAdjunto(apiLienzo.current, adjunto, dataURL, ancho, alto)
        await vaciar()
      } catch (err) {
        mostrar(err.message) // no se añadió nada
      }
    }
  }

  function interceptar(e) {
    const dt = e.clipboardData ?? e.dataTransfer
    const archivos = archivosDe(dt)
    if (!archivos.length && !traeArchivos(dt)) return // texto u otro pegado: pasa intacto
    e.preventDefault()
    e.stopPropagation()
    if (archivos.length) adjuntar(archivos)
  }

  // Segunda barrera: un pegado interno que arrastre archivos o un adjunto ajeno no se aplica.
  const alPegar = (datos) =>
    !Object.keys(datos?.files ?? {}).length &&
    !(datos?.elements ?? []).some((e) => {
      const sha = shaDeElemento(e)
      return sha != null && !conocidos.current?.has(sha)
    })

  return (
    <div className="term-lienzo" onDropCapture={interceptar} onPasteCapture={interceptar}>
      <button type="button" className="btn-outline term-adjuntar" title={TEXTO_PIZARRA.adjuntarAyuda} onClick={() => selector.current?.click()}>
        {TEXTO_PIZARRA.adjuntar}
      </button>
      <input
        ref={selector} type="file" multiple hidden
        onChange={(e) => { const lista = Array.from(e.target.files ?? []); e.target.value = ''; if (lista.length) adjuntar(lista) }}
      />
      <Excalidraw
        excalidrawAPI={(a) => { apiLienzo.current = a; alTenerApi(a) }}
        initialData={{ elements: inicial.elements, appState: inicial.appState, files: archivos, scrollToContent: true }}
        UIOptions={UI_OPCIONES}
        aiEnabled={false}
        theme="dark"
        onPaste={alPegar}
        langCode="es-ES"
        onChange={(elements, appState) => alCambiar(sanearEscena(elements, appState, conocidos.current), `${versionEscena(elements)}|${appState.viewBackgroundColor}`)}
      />
      {aviso && <div className="term-aviso">{aviso}</div>}
    </div>
  )
}
