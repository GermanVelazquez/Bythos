import { TEXTO_PIZARRA } from '../api.js'

// Presentacional: chip del estado de la terminal y chip de guardado de la pizarra.
export function ChipTerminal({ clave, texto }) {
  return <span className={`term-chip term-chip-${clave}`}>{texto}</span>
}

export default function ChipsEstado({ guardado }) {
  return <span className={`term-chip term-guardado-${guardado}`}>{TEXTO_PIZARRA[guardado]}</span>
}
