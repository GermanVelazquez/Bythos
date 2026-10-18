// conexionPty.js — ticket + WebSocket de una sesión de Terminal.
// Protocolo (api/terminal.go): binario = bytes de la terminal en ambos sentidos;
// texto cliente->servidor {tipo:'resize', cols, filas}; texto servidor->cliente
// {tipo:'salida', codigo}. Cierre 4000 = otra ventana tomó la sesión (sin
// reintento); 4001 = cliente lento (se reconecta).
import { api } from '../api.js'

const ESPERA_MIN = 500
const ESPERA_MAX = 5000
const CIERRE_REEMPLAZADA = 4000

// Devuelve { enviar(texto), redimensionar(cols, filas), cerrar() }.
// Callbacks: alDatos(Uint8Array), alReiniciar() (antes de cada repetición),
// alEstado(estado, extra). Estados: conectando, conectado, reconectando,
// terminado (extra = código), reemplazada, sinconpty, limite.
export function conectarPty(sesionId, { alDatos, alReiniciar, alEstado }) {
  const codificar = new TextEncoder()
  let ws = null
  let espera = ESPERA_MIN
  let temporizador = null
  let cerrado = false
  let ultimo = null // último tamaño {cols, filas}

  async function diagnosticar() {
    // El upgrade fallido no deja ver el status HTTP (501/429): lo deducimos del entorno.
    try {
      const e = await api.entornoTerminal()
      if (e?.conpty === false) return 'sinconpty'
      if ((e?.sesiones ?? 0) >= 8) return 'limite'
    } catch { /* sin entorno: reintento normal */ }
    return null
  }

  function reintentar() {
    if (cerrado) return
    alEstado('reconectando')
    temporizador = setTimeout(abrir, espera)
    espera = Math.min(espera * 2, ESPERA_MAX)
  }

  async function abrir() {
    if (cerrado) return
    let ticket
    try {
      ticket = (await api.ticketTerminal(sesionId)).ticket
    } catch {
      return reintentar()
    }
    if (cerrado) return
    const esquema = location.protocol === 'https:' ? 'wss' : 'ws'
    const sock = new WebSocket(`${esquema}://${location.host}/api/terminal/sesiones/${sesionId}/ws?ticket=${encodeURIComponent(ticket)}`)
    sock.binaryType = 'arraybuffer'
    ws = sock
    let abierto = false
    let terminado = false

    sock.onopen = () => {
      abierto = true
      espera = ESPERA_MIN
      alReiniciar() // la repetición reemplaza lo que haya en pantalla
      alEstado('conectado')
      if (ultimo) sock.send(JSON.stringify({ tipo: 'resize', ...ultimo }))
    }
    sock.onmessage = (ev) => {
      if (typeof ev.data !== 'string') return alDatos(new Uint8Array(ev.data))
      try {
        const m = JSON.parse(ev.data)
        if (m.tipo === 'salida') { terminado = true; alEstado('terminado', m.codigo) }
      } catch { /* texto desconocido: se ignora */ }
    }
    sock.onclose = async (ev) => {
      if (cerrado || ws !== sock) return
      if (ev.code === CIERRE_REEMPLAZADA) return alEstado('reemplazada')
      if (terminado) return
      if (!abierto) {
        const motivo = await diagnosticar()
        if (motivo) return alEstado(motivo)
      }
      reintentar()
    }
  }

  alEstado('conectando')
  abrir()

  return {
    enviar(texto) {
      if (ws?.readyState === WebSocket.OPEN) ws.send(codificar.encode(texto))
    },
    redimensionar(cols, filas) {
      ultimo = { cols, filas }
      if (ws?.readyState === WebSocket.OPEN) ws.send(JSON.stringify({ tipo: 'resize', cols, filas }))
    },
    cerrar() {
      cerrado = true
      clearTimeout(temporizador)
      ws?.close()
    },
  }
}
