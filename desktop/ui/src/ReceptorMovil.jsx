import { useEffect, useState } from 'react'

// ReceptorMovil.jsx — pantalla "Recibir del celular" (presentacional).
// No hace fetch: App.jsx es el contenedor (polling, activar/desactivar,
// aprobar/rechazar) y le pasa estado + callbacks. Aquí solo hay UI y un
// temporizador local para la pista de "nadie se conectó todavía".

const SIN_REQUESTS_MS = 60_000

// Guía por código de error de cable (ver desktop/api/receptor.go) o por
// categoria_red "desconocida". `titulo` + `pasos` se muestran en el panel.
const GUIAS = {
  red_no_privada: {
    titulo: 'Marcar la red como Privada',
    pasos: [
      'Abre Configuración de Windows > Red e Internet.',
      'Entra en las propiedades de tu Wi-Fi o Ethernet.',
      'En "Tipo de perfil de red" elige Privada.',
      'Vuelve aquí y pulsa Reintentar.',
    ],
  },
  puerto_ocupado: {
    titulo: 'El puerto del receptor está en uso',
    pasos: [
      'Cierra otra ventana de Bythos o el programa que use el puerto 48080.',
      'Pulsa Reintentar.',
    ],
  },
  sin_interfaz: {
    titulo: 'No hay una red privada disponible',
    pasos: [
      'Conecta la PC al mismo Wi-Fi o red local que tu celular.',
      'Marca esa red como Privada en Windows.',
      'Pulsa Reintentar.',
    ],
  },
  desconocida: {
    titulo: 'No pudimos confirmar el tipo de red',
    pasos: [
      'Bythos no pudo comprobar si la red es Privada, así que no puede garantizar que sea de confianza.',
      'Si estás en tu red de casa, marca la red como Privada en Configuración de Windows.',
      'Si es una red pública o compartida, desactiva la recepción.',
    ],
  },
}

function GuiaRed({ codigo, mensaje, onReintentar, ocupado }) {
  const guia = GUIAS[codigo]
  if (!guia) return <p className="agente-feedback error" style={{ margin: '0 0 12px', fontSize: 12 }}>{mensaje}</p>
  return (
    <div className="guia-red">
      <p className="guia-red-title">{guia.titulo}</p>
      <ol className="guia-red-steps">
        {guia.pasos.map((p) => <li key={p}>{p}</li>)}
      </ol>
      {onReintentar && (
        <button className="btn-outline" onClick={onReintentar} disabled={ocupado}>Reintentar</button>
      )}
    </div>
  )
}

function fechaCorta(iso) {
  if (!iso) return 'Sin uso todavía'
  const d = new Date(iso.replace(' ', 'T'))
  return Number.isNaN(d.getTime()) ? iso : d.toLocaleString('es')
}

export default function ReceptorMovil({
  estado, error, ocupado,
  onActivar, onDesactivar, onRenovar, onAprobar, onRechazar, onRevocar,
}) {
  const [ip, setIp] = useState('')
  const [sinRequests, setSinRequests] = useState(false)
  const [copiado, setCopiado] = useState(false)

  // El texto del QR lleva IP, código de un solo uso y huella: se copia al
  // portapapeles sin mostrarlo en pantalla. Al renovar el código se reinicia.
  useEffect(() => { setCopiado(false) }, [estado?.qr_texto])
  const copiarCodigo = () => {
    navigator.clipboard.writeText(estado.qr_texto).then(() => setCopiado(true), () => setCopiado(false))
  }

  const activo = !!estado?.activo
  const sinActividad = activo && estado.dispositivos.length === 0 && estado.solicitudes.length === 0

  // Pista tras 60 s activo sin que nadie intente emparejar ni haya dispositivos.
  useEffect(() => {
    if (!sinActividad) { setSinRequests(false); return undefined }
    const t = setTimeout(() => setSinRequests(true), SIN_REQUESTS_MS)
    return () => clearTimeout(t)
  }, [sinActividad])

  if (!estado) {
    return <div className="panel"><p className="panel-sub">Comprobando el receptor…</p></div>
  }

  const interfaces = estado.interfaces
  const ipElegida = ip || interfaces[0]?.ip || ''

  return (
    <div>
      <div className="panel">
        <p className="panel-title">Recibir del celular</p>
        <p className="panel-sub">
          Envía links y archivos desde tu celular por la red local, sin salir a internet.
          La recepción se cierra sola a los 10 min sin emparejar ni usar, o 2 min después del último uso.
          Para un QR nuevo, vuelve a abrir «Recibir del celular».
        </p>

        {error && <GuiaRed codigo={error.codigo} mensaje={error.message} onReintentar={() => onActivar(ipElegida)} ocupado={ocupado} />}

        {!activo ? (
          <div className="inline-form">
            {interfaces.length > 1 && (
              <select value={ipElegida} onChange={(e) => setIp(e.target.value)} className="receptor-select">
                {interfaces.map((i) => <option key={i.ip} value={i.ip}>{i.nombre}</option>)}
              </select>
            )}
            <button className="btn-primary" disabled={ocupado} onClick={() => onActivar(interfaces.length > 1 ? ipElegida : '')}>
              {ocupado ? 'Activando…' : 'Activar recepción'}
            </button>
          </div>
        ) : (
          <>
            {estado.categoria_red === 'desconocida' && (
              <GuiaRed codigo="desconocida" mensaje="" onReintentar={null} />
            )}
            <div className="receptor-qr">
              {estado.qr_png && <img src={estado.qr_png} alt="Código QR para emparejar el celular" width="220" height="220" />}
              <div className="receptor-qr-info">
                <p className="panel-sub">Escanea el QR con la app de Bythos del celular. Sirve para un solo emparejamiento y vence en 10 min.</p>
                <p className="panel-sub">Si no puedes escanear, copia el código y pégalo en la app.</p>
                <div className="export-grid">
                  <button className="btn-outline" disabled={!estado.qr_texto} onClick={copiarCodigo}>
                    {copiado ? 'Código copiado' : 'Copiar código manual'}
                  </button>
                  <button className="btn-outline" disabled={ocupado} onClick={onRenovar}>Renovar código</button>
                  <button className="btn-outline" disabled={ocupado} onClick={onDesactivar}>Desactivar</button>
                </div>
              </div>
            </div>
            {sinRequests && (
              <p className="panel-sub receptor-hint">
                Pasó un minuto y ningún celular se conectó. Revisa que el celular esté en el mismo Wi-Fi,
                que la red de la PC sea Privada, que el firewall permita a Bythos y que el Wi-Fi no aísle los dispositivos.
              </p>
            )}
          </>
        )}
      </div>

      {estado.solicitudes.map((s) => (
        <div key={s.id} className="panel">
          <p className="panel-title">{s.nombre || 'Un celular'} quiere emparejarse</p>
          <p className="panel-sub">Aprueba solo si el celular muestra el mismo código de 6 dígitos.</p>
          <p className="receptor-sas">{s.sas}</p>
          <div className="export-grid">
            <button className="btn-primary" onClick={() => onAprobar(s.id)}>Aprobar</button>
            <button className="btn-outline" onClick={() => onRechazar(s.id)}>Rechazar</button>
          </div>
        </div>
      ))}

      {(estado.dispositivos.length > 0 || estado.transferencias.length > 0) && (
        <div className="panel">
          <p className="panel-title">Celulares emparejados</p>
          {estado.dispositivos.length === 0 && <p className="panel-sub">Ninguno.</p>}
          {estado.dispositivos.map((d) => (
            <div key={d.id} className="setting-row">
              <span className="setting-key">{d.nombre}</span>
              <span className="setting-val">
                {fechaCorta(d.ultimo_uso)}
                <button className="btn-outline" style={{ marginLeft: 12 }} onClick={() => onRevocar(d.id)}>Revocar</button>
              </span>
            </div>
          ))}
          {estado.transferencias.length > 0 && <p className="panel-title" style={{ marginTop: 16 }}>Transferencias</p>}
          {estado.transferencias.map((t) => (
            <div key={t.id} className="setting-row">
              <span className="setting-key">{t.nombre}</span>
              <span className="setting-val">
                {t.estado}{t.tamano > 0 ? ` · ${Math.min(100, Math.round((t.offset / t.tamano) * 100))}%` : ''}
              </span>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
