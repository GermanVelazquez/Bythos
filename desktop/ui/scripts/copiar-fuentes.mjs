// Copia las fuentes de Excalidraw a public/excalidraw/fonts para servirlas
// desde el propio .exe (sin CDN). Se excluye Xiaolai (CJK, 13 MB de 14).
// La carpeta de salida está en .gitignore: se regenera desde el lockfile.
import { cpSync, rmSync } from 'node:fs'
import { fileURLToPath } from 'node:url'

const raiz = new URL('..', import.meta.url)
const origen = fileURLToPath(new URL('node_modules/@excalidraw/excalidraw/dist/prod/fonts', raiz))
const destino = fileURLToPath(new URL('public/excalidraw/fonts', raiz))

rmSync(destino, { recursive: true, force: true })
cpSync(origen, destino, { recursive: true, filter: (ruta) => !/xiaolai/i.test(ruta) })
console.log('Fuentes de Excalidraw copiadas (sin Xiaolai):', destino)
