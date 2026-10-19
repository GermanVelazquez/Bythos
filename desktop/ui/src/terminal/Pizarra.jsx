import { useEffect, useRef, useState } from 'react'
import { Excalidraw, sanearEscena, versionEscena, insertarAdjunto, shaDeElemento, shaDeEnlace, adjuntoSeleccionado, quitarElementos } from './excalidrawAdapter.js'
import { subirArchivo, leerComoDataURL } from './adjuntos.js'
import { api, TEXTO_PIZARRA } from '../api.js'
import { FileViewerModal } from '../Archivos.jsx'
import BarraAdjunto from './BarraAdjunto.jsx'
import SelectorRecursoCarpeta from './SelectorRecursoCarpeta.jsx'

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
// `carpetaId` (de la sesión, null si no tiene) habilita "Guardar en carpeta" y el selector de recursos.
export default function Pizarra({ sesionId, carpetaId, inicial, archivos, conocidosIniciales, alCambiar, alTenerApi, vaciar }) {
  const [aviso, setAviso] = useState('')
  const [seleccion, setSeleccion] = useState(null) // adjunto bajo la selección (barra de acciones)
  const [visor, setVisor] = useState(null) // recurso sintético para FileViewerModal
  const [recursos, setRecursos] = useState(null) // null = selector cerrado
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

  // La ficha del adjunto se busca en la lista viva: el agente también puede haber añadido alguno.
  async function adjuntoDe(sha) {
    const ficha = (await api.listarAdjuntos(sesionId)).find((a) => a.sha256 === sha)
    if (!ficha) throw new Error(TEXTO_PIZARRA.adjuntoNoDisponible)
    return ficha
  }

  async function abrir(sha) {
    try {
      const a = await adjuntoDe(sha)
      setVisor({ tipo: a.tipo, titulo: a.nombre, archivo: { id: a.id, nombre: a.nombre, tamano: 0 } })
    } catch (err) { mostrar(err.message) }
  }

  async function guardarEnCarpeta(sha) {
    try {
      const { recurso, existia } = await api.guardarAdjuntoEnCarpeta(sesionId, (await adjuntoDe(sha)).id)
      mostrar(existia ? TEXTO_PIZARRA.yaEnCarpeta(recurso.Titulo) : TEXTO_PIZARRA.guardadoEnCarpeta)
    } catch (err) { mostrar(err.message) }
  }

  async function abrirSelector() {
    try {
      const todos = await api.listarRecursos(carpetaId)
      setRecursos((todos ?? []).filter((r) => r.Archivo && r.Tipo !== 'video' && !r.Archivo.Mime?.startsWith('video/')))
    } catch (err) { mostrar(err.message) }
  }

  async function adjuntarRecurso(recurso) {
    setRecursos(null)
    try {
      const adjunto = await api.adjuntarDesdeRecurso(sesionId, recurso.ID)
      let dataURL = null
      let ancho = 0
      let alto = 0
      if (adjunto.tipo === 'imagen') {
        const bytes = await api.bytesAdjunto(sesionId, adjunto.id)
        dataURL = await leerComoDataURL(bytes)
        try { const bmp = await createImageBitmap(bytes); ancho = bmp.width; alto = bmp.height; bmp.close() } catch { /* sin medir */ }
      }
      conocidos.current?.add(adjunto.sha256)
      insertarAdjunto(apiLienzo.current, adjunto, dataURL, ancho, alto)
      await vaciar()
    } catch (err) { mostrar(err.message) }
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
      {carpetaId != null && (
        <button type="button" className="btn-outline term-adjuntar term-adjuntar-carpeta" onClick={abrirSelector}>{TEXTO_PIZARRA.deCarpeta}</button>
      )}
      {recursos && <SelectorRecursoCarpeta recursos={recursos} alElegir={adjuntarRecurso} alCerrar={() => setRecursos(null)} />}
      {seleccion && (
        <BarraAdjunto
          puedeGuardar={carpetaId != null}
          alAbrir={() => abrir(seleccion.sha)}
          alGuardar={() => guardarEnCarpeta(seleccion.sha)}
          alQuitar={() => { quitarElementos(apiLienzo.current, seleccion.ids); setSeleccion(null) }}
        />
      )}
      <FileViewerModal
        resource={visor} onClose={() => setVisor(null)}
        urlContenido={(id) => api.urlContenidoAdjunto(sesionId, id)} leerTexto={(id) => api.textoAdjunto(sesionId, id)}
      />
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
        onLinkOpen={(el, ev) => {
          const sha = shaDeEnlace(el.link)
          if (sha == null) return // enlace ajeno: Excalidraw hace lo suyo
          ev.preventDefault() // el enlace interno nunca navega
          abrir(sha)
        }}
        onChange={(elements, appState) => {
          const s = adjuntoSeleccionado(elements, appState)
          setSeleccion((previa) => (previa?.sha === s?.sha && previa?.ids.join() === s?.ids.join() ? previa : s))
          alCambiar(sanearEscena(elements, appState, conocidos.current), `${versionEscena(elements)}|${appState.viewBackgroundColor}`)
        }}
      />
      {aviso && <div className="term-aviso">{aviso}</div>}
    </div>
  )
}
