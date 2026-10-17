import { useEffect, useState } from 'react'
import { api } from './api.js'
import { Icon } from './App.jsx'

// Archivos.jsx — Paso 0: subir y ver ARCHIVOS (PDF, video, imagen,
// documento) como recursos, no solo links. Componentes separados de
// App.jsx (que ya es grande) a propósito: el modal de subida y el
// visor son autocontenidos, App.jsx solo los monta y les pasa estado.

// formatoTamano muestra bytes legibles ("2.4 MB") en el picker y en el
// visor. Sin librería: 4 unidades alcanzan de sobra para PDFs/videos.
function formatoTamano(bytes) {
  if (!bytes) return '0 B'
  const unidades = ['B', 'KB', 'MB', 'GB']
  let n = bytes
  let i = 0
  while (n >= 1024 && i < unidades.length - 1) {
    n /= 1024
    i++
  }
  return `${i === 0 ? n : n.toFixed(1)} ${unidades[i]}`
}

// EXTENSIONES_ACEPTADAS es solo una pista para el picker del SO (filtra
// qué se ve más fácil de elegir); el whitelist real es por CONTENIDO en
// el backend (ver desktop/archivos), así que esto nunca es la única
// defensa — un archivo con extensión falsa igual lo rechaza el backend.
const EXTENSIONES_ACEPTADAS = '.pdf,.mp4,.m4v,.mov,.webm,.mkv,.png,.jpg,.jpeg,.gif,.webp,.docx,.xlsx,.pptx,.txt,.md'

/* ───────── ARCHIVO PREVIEW (tarjetas) ───────── */
// ArchivoPreview decide qué mostrar en el thumb de una tarjeta de
// recurso tipo archivo, SIN trabajo de servidor salvo para imagen
// (miniatura JPEG cacheada, ver GET /archivos/{id}/miniatura):
// - imagen: la miniatura; si 404 (webp, tipo sin soporte, decodificación
//   fallida), onError la oculta y el thumb-badge de siempre (ya en la
//   tarjeta, ver ResourceCard en App.jsx) queda como único indicador —
//   mismo resultado visual que "cae al ícono de tipo".
// - video: primer cuadro del propio archivo con <video preload="metadata"
//   muted> + #t=0.1 (Media Fragments URI: le pide al navegador el cuadro
//   en el segundo 0.1), sin controles — la tarjeta no es el reproductor.
// - pdf/documento: sin preview, el thumb-badge alcanza.
export function ArchivoPreview({ resource }) {
  const archivo = resource.archivo
  if (!archivo) return null
  if (resource.tipo === 'imagen') {
    return (
      <img
        src={api.miniaturaArchivoUrl(archivo.id)}
        alt=""
        loading="lazy"
        onError={(e) => { e.currentTarget.style.display = 'none' }}
      />
    )
  }
  if (resource.tipo === 'video') {
    return (
      <video
        className="thumb-video"
        src={`${api.contenidoArchivoUrl(archivo.id)}#t=0.1`}
        preload="metadata"
        muted
        aria-hidden="true"
      />
    )
  }
  return null
}

/* ───────── ADD FILE MODAL ───────── */
export function AddFileModal({ open, onClose, onUpload, carpetas, presetFile }) {
  const [file, setFile] = useState(null)
  const [carpetaId, setCarpetaId] = useState('')
  const [subiendo, setSubiendo] = useState(false)
  const [progreso, setProgreso] = useState(0)
  const [error, setError] = useState('')

  // Reset limpio cada vez que se abre/cierra, y toma el File que vino
  // por drag&drop (si vino) como preselección.
  useEffect(() => {
    if (!open) return
    setFile(presetFile ?? null)
    setProgreso(0)
    setError('')
    setSubiendo(false)
  }, [open, presetFile])

  useEffect(() => {
    if (open && carpetas.length > 0 && !carpetaId) setCarpetaId(String(carpetas[0].id))
    if (open && carpetas.length === 0) setCarpetaId('')
  }, [open, carpetas]) // eslint-disable-line react-hooks/exhaustive-deps

  if (!open) return null

  const handleSubmit = async (e) => {
    e.preventDefault()
    if (!file || !carpetaId || subiendo) return
    setError('')
    setSubiendo(true)
    try {
      await onUpload(file, Number(carpetaId), setProgreso)
      onClose()
    } catch (err) {
      setError(err.message || 'No se pudo subir el archivo')
      setSubiendo(false)
    }
  }

  return (
    <div className="modal-overlay animate-fade-in" onClick={subiendo ? undefined : onClose}>
      <form className="modal animate-slide-up" onClick={(e) => e.stopPropagation()} onSubmit={handleSubmit}>
        <div className="modal-head">
          <h2 className="modal-title">Añadir archivo</h2>
          <button type="button" className="modal-close" onClick={onClose} disabled={subiendo}>
            <Icon name="x" size={14} />
          </button>
        </div>
        <div className="field">
          <label className="field-label">Archivo *</label>
          <input
            type="file"
            accept={EXTENSIONES_ACEPTADAS}
            disabled={subiendo}
            onChange={(e) => setFile(e.target.files?.[0] ?? null)}
            required
          />
          {file && <p className="field-hint">{file.name} · {formatoTamano(file.size)}</p>}
          <p className="field-hint">PDF, video, imagen o documento de Office/texto. Bythos detecta el tipo por contenido, no confía en la extensión.</p>
        </div>
        <div className="field">
          <label className="field-label">Carpeta *</label>
          {carpetas.length === 0 ? (
            <p className="field-hint">Crea primero una carpeta en la vista Carpetas.</p>
          ) : (
            <select value={carpetaId} onChange={(e) => setCarpetaId(e.target.value)} disabled={subiendo} required>
              {carpetas.map((c) => (
                <option key={c.id} value={c.id}>{c.nombre}</option>
              ))}
            </select>
          )}
        </div>
        {subiendo && (
          <div className="field">
            <div className="bar"><div style={{ background: '#5FA97B', width: `${progreso}%` }} /></div>
            <p className="field-hint">Subiendo… {progreso}%</p>
          </div>
        )}
        {error && <p className="field-error">{error}</p>}
        <div className="modal-actions">
          <button type="button" className="btn-ghost" onClick={onClose} disabled={subiendo}>Cancelar</button>
          <button type="submit" className="btn-solid" disabled={!file || !carpetaId || subiendo}>
            {subiendo ? 'Subiendo…' : 'Subir archivo'}
          </button>
        </div>
      </form>
    </div>
  )
}

/* ───────── DOC PREVIEW (Office/texto, dentro del visor) ───────── */
// renderBloquesDocx agrupa los bloques "lista" consecutivos en un <ul>
// (HTML válido: un <li> nunca puede ir suelto sin su lista contenedora),
// y arma <h1>..<h6> para "titulo" según el nivel que mandó el backend.
function renderBloquesDocx(bloques) {
  const out = []
  let listaBuffer = []
  const flushLista = (key) => {
    if (listaBuffer.length === 0) return
    out.push(<ul className="doc-list" key={`ul-${key}`}>{listaBuffer}</ul>)
    listaBuffer = []
  }
  bloques.forEach((b, i) => {
    if (b.tipo === 'lista') {
      listaBuffer.push(<li key={i} className="doc-list-item">{b.texto}</li>)
      return
    }
    flushLista(i)
    if (b.tipo === 'titulo') {
      const nivel = Math.min(6, Math.max(1, b.nivel || 1))
      const Tag = `h${nivel}`
      out.push(<Tag key={i} className="doc-heading">{b.texto}</Tag>)
    } else {
      out.push(<p key={i} className="doc-paragraph">{b.texto}</p>)
    }
  })
  flushLista('final')
  return out
}

// DocPreview renderiza lo que devolvió GET /archivos/{id}/texto según su
// "tipo". El texto plano (rama por default, cubre "texto" = txt/md)
// SIEMPRE se muestra como TEXTO en un <pre> — nunca como HTML, ni
// siquiera el .md: es una vista simple, no un renderer de markdown, y
// meter dangerouslySetInnerHTML acá sería inyección de HTML servida por
// el propio usuario.
function DocPreview({ texto }) {
  if (texto.tipo === 'docx') {
    return <div className="doc-preview doc-preview-docx">{renderBloquesDocx(texto.bloques || [])}</div>
  }
  if (texto.tipo === 'pptx') {
    return (
      <div className="doc-preview doc-preview-pptx">
        {(texto.diapositivas || []).map((d) => (
          <section key={d.numero} className="doc-slide">
            <h3 className="doc-slide-title">Diapositiva {d.numero}</h3>
            {(d.parrafos || []).map((p, i) => <p key={i} className="doc-paragraph">{p}</p>)}
          </section>
        ))}
      </div>
    )
  }
  if (texto.tipo === 'xlsx') {
    return (
      <div className="doc-preview doc-preview-xlsx">
        <table className="doc-table">
          <tbody>
            {(texto.filas || []).map((fila, i) => (
              <tr key={i}>
                {fila.map((celda, j) => <td key={j}>{celda}</td>)}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    )
  }
  return <pre className="doc-plain">{texto.texto}</pre>
}

/* ───────── FILE VIEWER MODAL ───────── */
// Abre PDF/video/imagen en la app (iframe/video/img contra /contenido).
// Documentos de Office/texto piden GET /archivos/{id}/texto y muestran
// una vista de texto simple (DocPreview) en vez de solo Descargar;
// abrirlos con el programa del sistema significaría lanzar un proceso —
// fuera de alcance de "Paso 0", ver instrucciones — así que Descargar
// sigue siempre disponible como respaldo (formato exacto, garantizado).
export function FileViewerModal({ resource, onClose }) {
  const [textoDoc, setTextoDoc] = useState(null)
  const [textoCargando, setTextoCargando] = useState(false)

  useEffect(() => {
    if (!resource) return
    const onKey = (e) => { if (e.key === 'Escape') onClose() }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [resource, onClose])

  // Solo los recursos "documento" (docx/xlsx/pptx/txt/md, ver TipoRecurso
  // en Go) tienen vista de texto; pdf/video/imagen ya tienen su propio
  // visor nativo más arriba y nunca llaman a /texto.
  useEffect(() => {
    setTextoDoc(null)
    if (!resource || resource.tipo !== 'documento' || !resource.archivo) return
    let cancelado = false
    setTextoCargando(true)
    api.textoArchivo(resource.archivo.id)
      .then((data) => { if (!cancelado) setTextoDoc(data) })
      .catch(() => { /* 422 u otro error: textoDoc queda null, cae a Descargar */ })
      .finally(() => { if (!cancelado) setTextoCargando(false) })
    return () => { cancelado = true }
  }, [resource])

  if (!resource || !resource.archivo) return null

  const src = api.contenidoArchivoUrl(resource.archivo.id)
  const titulo = resource.titulo || resource.archivo.nombre

  let cuerpo
  if (resource.tipo === 'pdf') {
    cuerpo = <iframe className="viewer-frame" src={src} title={titulo} />
  } else if (resource.tipo === 'video') {
    // eslint-disable-next-line jsx-a11y/media-has-caption
    cuerpo = <video className="viewer-video" src={src} controls autoPlay />
  } else if (resource.tipo === 'imagen') {
    cuerpo = <img className="viewer-image" src={src} alt={titulo} />
  } else if (textoCargando) {
    cuerpo = (
      <div className="viewer-doc">
        <Icon name="file" size={28} />
        <p>Generando vista previa…</p>
      </div>
    )
  } else if (textoDoc) {
    cuerpo = (
      <div className="viewer-doc viewer-doc-preview">
        <p className="field-hint doc-preview-note">Vista de texto: el formato original puede verse distinto.</p>
        <DocPreview texto={textoDoc} />
        {textoDoc.truncado && <p className="field-hint">Se recortó el documento por ser muy largo.</p>}
        <a className="btn-solid" href={src} download={resource.archivo.nombre}>
          <Icon name="download" size={12} /> Descargar
        </a>
      </div>
    )
  } else {
    cuerpo = (
      <div className="viewer-doc">
        <Icon name="file" size={28} />
        <p>Bythos no puede previsualizar este tipo de documento acá.</p>
        <p className="field-hint">{resource.archivo.nombre} · {formatoTamano(resource.archivo.tamano)}</p>
        <a className="btn-solid" href={src} download={resource.archivo.nombre}>
          <Icon name="download" size={12} /> Descargar
        </a>
      </div>
    )
  }

  return (
    <div className="modal-overlay animate-fade-in" onClick={onClose}>
      <div className="modal viewer-modal animate-slide-up" onClick={(e) => e.stopPropagation()}>
        <div className="modal-head">
          <h2 className="modal-title line-clamp-2">{titulo}</h2>
          <button type="button" className="modal-close" onClick={onClose} autoFocus>
            <Icon name="x" size={14} />
          </button>
        </div>
        <div className="viewer-body">{cuerpo}</div>
      </div>
    </div>
  )
}
