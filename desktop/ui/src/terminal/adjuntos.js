// Adjuntos de la pizarra, lado cliente: prefiltro de tipo y tamaño (los mismos
// topes que el servidor, que decide por contenido), reducción de imágenes y
// carga. Sin Excalidraw: solo funciones de archivo, fáciles de probar.
import { api } from '../api.js'

export const LIMITE_BYTES = 25 * 1024 * 1024
export const LADO_MAXIMO = 1440

export const TEXTO_ADJUNTO = {
  tipoNoPermitido: 'Tipo de archivo no permitido. La pizarra acepta imágenes (png/jpg/gif/webp), PDF, documentos de Office (docx/xlsx/pptx) y texto plano (txt/md).',
  video: 'Los videos no se pueden adjuntar a la pizarra.',
  demasiado: 'El archivo es demasiado grande (máximo 25 MiB).',
  fallo: 'No se pudo adjuntar el archivo.',
}

const EXT_PERMITIDAS = new Set(['png', 'jpg', 'jpeg', 'gif', 'webp', 'pdf', 'docx', 'xlsx', 'pptx', 'txt', 'md'])
const EXT_VIDEO = new Set(['mp4', 'webm', 'mkv', 'mov'])
// Capturas del portapapeles llegan sin nombre útil: se deduce de su MIME.
const EXT_DE_MIME = { 'image/png': 'png', 'image/jpeg': 'jpg', 'image/gif': 'gif', 'image/webp': 'webp' }

const extension = (nombre) => (nombre?.includes('.') ? nombre.split('.').pop().toLowerCase() : '')

// '' si el archivo puede subirse; si no, el mensaje en español para mostrar.
export function validarArchivo(archivo) {
  const ext = extension(archivo.name) || EXT_DE_MIME[archivo.type] || ''
  if (EXT_VIDEO.has(ext) || archivo.type?.startsWith('video/')) return TEXTO_ADJUNTO.video
  if (!EXT_PERMITIDAS.has(ext)) return TEXTO_ADJUNTO.tipoNoPermitido
  if (archivo.size > LIMITE_BYTES) return TEXTO_ADJUNTO.demasiado
  return ''
}

// Dimensiones tras reducir el lado mayor a `maximo`; sin ampliar nunca.
export function dimensionesReducidas(ancho, alto, maximo = LADO_MAXIMO) {
  const mayor = Math.max(ancho, alto)
  if (mayor <= maximo) return { ancho, alto, reducida: false }
  const f = maximo / mayor
  return { ancho: Math.max(1, Math.round(ancho * f)), alto: Math.max(1, Math.round(alto * f)), reducida: true }
}

// Formato de salida al recodificar: JPEG 0.9 para jpeg, PNG para el resto.
const salidaDe = (mime) => (mime === 'image/jpeg' ? { mime: 'image/jpeg', ext: 'jpg', calidad: 0.9 } : { mime: 'image/png', ext: 'png' })

const conExtension = (nombre, ext) => `${(nombre || 'imagen').replace(/\.[^./\\]+$/, '')}.${ext}`

// Devuelve { archivo, ancho, alto } para una imagen (File ya validado): se
// reduce si pasa de 1440 px y WebP se recodifica siempre a PNG. Una imagen
// pequeña sube tal cual. Si el navegador no puede decodificarla, sube sin medir.
export async function prepararImagen(archivo) {
  let bmp
  try { bmp = await createImageBitmap(archivo) } catch { return { archivo, ancho: 0, alto: 0 } }
  try {
    const d = dimensionesReducidas(bmp.width, bmp.height)
    if (!d.reducida && archivo.type !== 'image/webp') return { archivo, ancho: d.ancho, alto: d.alto }
    const lienzo = document.createElement('canvas')
    lienzo.width = d.ancho
    lienzo.height = d.alto
    lienzo.getContext('2d').drawImage(bmp, 0, 0, d.ancho, d.alto)
    const salida = salidaDe(archivo.type)
    const blob = await new Promise((ok) => lienzo.toBlob(ok, salida.mime, salida.calidad))
    if (!blob) return { archivo, ancho: d.ancho, alto: d.alto }
    return { archivo: new File([blob], conExtension(archivo.name, salida.ext), { type: salida.mime }), ancho: d.ancho, alto: d.alto }
  } finally {
    bmp.close()
  }
}

export const leerComoDataURL = (blob) => new Promise((ok, ko) => {
  const r = new FileReader()
  r.onload = () => ok(r.result)
  r.onerror = () => ko(r.error)
  r.readAsDataURL(blob)
})

// Valida, reduce (si es imagen) y sube. Lanza Error con el mensaje a mostrar.
// Devuelve { adjunto, archivo, ancho, alto }: `archivo` es lo realmente subido.
export async function subirArchivo(sesionId, original) {
  const problema = validarArchivo(original)
  if (problema) throw new Error(problema)
  const esImagen = original.type?.startsWith('image/') || EXT_DE_MIME[original.type]
  const { archivo, ancho, alto } = esImagen ? await prepararImagen(original) : { archivo: original, ancho: 0, alto: 0 }
  const adjunto = await api.subirAdjunto(sesionId, archivo)
  return { adjunto, archivo, ancho, alto }
}
