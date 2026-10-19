import { TEXTO_PIZARRA } from '../api.js'

// Presentacional: recursos de archivo de la carpeta de la sesión (sin videos). El padre decide
// cuándo mostrarlo (solo con carpeta) y trae la lista; elegir uno sale por `alElegir`.
// Recursos tal como los manda Go (sin normalizar): ID, Titulo, Tipo, Archivo.
export default function SelectorRecursoCarpeta({ recursos, alElegir, alCerrar }) {
  return (
    <div className="term-selector-recurso" role="menu">
      {recursos.length === 0 && <p className="field-hint">{TEXTO_PIZARRA.carpetaVacia}</p>}
      {recursos.map((r) => (
        <button key={r.ID} type="button" role="menuitem" className="term-selector-item" title={r.Titulo} onClick={() => alElegir(r)}>
          {r.Titulo || r.Archivo?.NombreOriginal}
        </button>
      ))}
      <button type="button" className="btn-outline" onClick={alCerrar}>Cerrar</button>
    </div>
  )
}
