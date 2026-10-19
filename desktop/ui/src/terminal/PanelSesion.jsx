import { lazy, Suspense, useRef, useState } from 'react'
import PanelTerminal from './PanelTerminal.jsx'
import ChipsEstado from './ChipsEstado.jsx'
import { useAutoguardado } from './useAutoguardado.js'
import { usePizarraAgente } from './usePizarraAgente.js'
import { TEXTO_PIZARRA } from '../api.js'

// Excalidraw va en su propio chunk: solo se pide al abrir una sesión.
const Pizarra = lazy(() => import('./Pizarra.jsx'))

// Una sesión: terminal a la izquierda, pizarra a la derecha. Montado solo para la
// pestaña activa; al desmontar el hook vacía el autoguardado pendiente.
export default function PanelSesion({ sesion }) {
  const { carga, guardado, aviso, alCambiar, vaciar } = useAutoguardado(sesion.id)
  const enfocar = useRef(null)
  const [apiLienzo, setApiLienzo] = useState(null) // API de Excalidraw; null mientras no hay lienzo
  usePizarraAgente(sesion.id, apiLienzo, vaciar)

  // Revisar: guarda lo pendiente y devuelve el foco a la terminal; no escribe nada al PTY.
  async function revisar() {
    await vaciar()
    enfocar.current?.()
  }

  return (
    <div className="term-sesion">
      <PanelTerminal sesion={sesion} enfocarRef={enfocar} />
      <section className="term-pizarra">
        <div className="term-estado">
          <ChipsEstado guardado={guardado} />
          <button className="btn-outline" onClick={revisar}>{TEXTO_PIZARRA.revisar}</button>
          {aviso && <span className="field-hint">{aviso}</span>}
        </div>
        {carga && (
          <Suspense fallback={null}>
            <Pizarra key={carga.n} sesionId={sesion.id} vaciar={vaciar} inicial={carga.escena} alCambiar={alCambiar} alTenerApi={setApiLienzo} />
          </Suspense>
        )}
      </section>
    </div>
  )
}
