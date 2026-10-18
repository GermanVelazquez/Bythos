import { lazy, Suspense, useRef } from 'react'
import PanelTerminal from './PanelTerminal.jsx'
import ChipsEstado from './ChipsEstado.jsx'
import { useAutoguardado } from './useAutoguardado.js'
import { TEXTO_PIZARRA } from '../api.js'

// Excalidraw va en su propio chunk: solo se pide al abrir una sesión.
const Pizarra = lazy(() => import('./Pizarra.jsx'))

// Una sesión: terminal a la izquierda, pizarra a la derecha. Montado solo para la
// pestaña activa; al desmontar el hook vacía el autoguardado pendiente.
export default function PanelSesion({ sesion }) {
  const { carga, guardado, aviso, alCambiar, vaciar } = useAutoguardado(sesion.id)
  const enfocar = useRef(null)

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
            <Pizarra key={carga.n} inicial={carga.escena} alCambiar={alCambiar} />
          </Suspense>
        )}
      </section>
    </div>
  )
}
