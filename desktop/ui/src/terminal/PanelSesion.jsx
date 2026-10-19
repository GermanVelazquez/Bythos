import { lazy, Suspense, useRef, useState } from 'react'
import PanelTerminal from './PanelTerminal.jsx'
import ChipsEstado from './ChipsEstado.jsx'
import { useAutoguardado } from './useAutoguardado.js'
import { usePizarraAgente } from './usePizarraAgente.js'
import { pedidoRevisar } from './conexionPty.js'
import { TEXTO_PIZARRA } from '../api.js'

// Excalidraw va en su propio chunk: solo se pide al abrir una sesión.
const Pizarra = lazy(() => import('./Pizarra.jsx'))

// Una sesión: terminal a la izquierda, pizarra a la derecha. Montado solo para la
// pestaña activa; al desmontar el hook vacía el autoguardado pendiente.
export default function PanelSesion({ sesion }) {
  const { carga, guardado, aviso, alCambiar, vaciar } = useAutoguardado(sesion.id)
  const enfocar = useRef(null)
  const escribir = useRef(null)
  const [avisoRevisar, setAvisoRevisar] = useState('')
  const [apiLienzo, setApiLienzo] = useState(null) // API de Excalidraw; null mientras no hay lienzo
  usePizarraAgente(sesion.id, apiLienzo, vaciar)

  // Revisar: guarda lo pendiente, pega el pedido (una línea, sin Enter) y devuelve el foco
  // a la terminal. Sin conexión no escribe nada y lo avisa. Nunca se vuelca contenido de la escena.
  async function revisar() {
    await vaciar()
    const pedido = pedidoRevisar(sesion.id)
    const escrito = pedido ? escribir.current?.(pedido) === true : false
    setAvisoRevisar(escrito ? '' : TEXTO_PIZARRA.revisarSinTerminal)
    enfocar.current?.()
  }

  return (
    <div className="term-sesion">
      <PanelTerminal sesion={sesion} enfocarRef={enfocar} escribirRef={escribir} />
      <section className="term-pizarra">
        <div className="term-estado">
          <ChipsEstado guardado={guardado} />
          <button className="btn-outline" onClick={revisar}>{TEXTO_PIZARRA.revisar}</button>
          {aviso && <span className="field-hint">{aviso}</span>}
          {avisoRevisar && <span className="field-hint">{avisoRevisar}</span>}
        </div>
        {carga && (
          <Suspense fallback={null}>
            <Pizarra key={carga.n} sesionId={sesion.id} carpetaId={sesion.carpeta_id ?? null}vaciar={vaciar} inicial={carga.escena} archivos={carga.files} conocidosIniciales={carga.conocidos} alCambiar={alCambiar} alTenerApi={setApiLienzo} />
          </Suspense>
        )}
      </section>
    </div>
  )
}
