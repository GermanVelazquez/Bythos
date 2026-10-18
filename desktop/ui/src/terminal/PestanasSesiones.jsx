import { TEXTO_TERMINAL } from '../api.js'

export const MODOS = {
  practica: 'Práctica', revision: 'Revisión', prototipo: 'Prototipo', dibujo: 'Dibujo', paginas: 'Páginas',
}

// Presentacional: pestañas de las sesiones abiertas. Cerrar una pestaña no borra la sesión.
export default function PestanasSesiones({ sesiones, activa, nombreCarpeta, onElegir, onCerrar, onNueva }) {
  return (
    <div className="term-tabs">
      {sesiones.map((s) => (
        <div key={s.id} className={`term-tab${s.id === activa ? ' active' : ''}`} onClick={() => onElegir(s.id)}>
          <span className="term-tab-nombre">{s.nombre}</span>
          <span className="term-tab-meta">
            {MODOS[s.modo] ?? s.modo} · {nombreCarpeta(s) || TEXTO_TERMINAL.suelta}
          </span>
          <button
            className="term-tab-x" title="Cerrar pestaña"
            onClick={(e) => { e.stopPropagation(); onCerrar(s.id) }}
          >×</button>
        </div>
      ))}
      <button className="btn-outline term-tab-nueva" onClick={onNueva}>+ Nueva sesión</button>
    </div>
  )
}
