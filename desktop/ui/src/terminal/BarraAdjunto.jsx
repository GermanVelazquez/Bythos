import { TEXTO_PIZARRA } from '../api.js'

// Presentacional: acciones del adjunto seleccionado (imagen o tarjeta). "Guardar en carpeta"
// solo existe si la sesión tiene carpeta. Todo sale por callbacks.
export default function BarraAdjunto({ puedeGuardar, alAbrir, alGuardar, alQuitar }) {
  return (
    <div className="term-barra-adjunto" role="toolbar">
      <button type="button" className="btn-outline" onClick={alAbrir}>{TEXTO_PIZARRA.abrir}</button>
      {puedeGuardar && <button type="button" className="btn-outline" onClick={alGuardar}>{TEXTO_PIZARRA.guardarEnCarpeta}</button>}
      <button type="button" className="btn-outline" onClick={alQuitar}>{TEXTO_PIZARRA.quitar}</button>
    </div>
  )
}
