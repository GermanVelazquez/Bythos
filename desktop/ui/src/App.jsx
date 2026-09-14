import { useEffect, useMemo, useState } from 'react'
import { api } from './api.js'

// App.jsx — Bythos desktop UI: sidebar + topbar + cards + modal (dark theme).
// Single-file layout on purpose: the whole product flow fits here.
// Data comes ONLY from api.js (Go backend); no mocks, no localStorage.

// Go structs have no `json:` tags, so keys arrive capitalized
// (ID, Titulo, Tipo, Estado...). Normalizers accept both cases.
const normCarpeta = (c) => ({
  id: c.ID ?? c.id,
  nombre: c.Nombre ?? c.nombre ?? c.name ?? '',
})

const normRecurso = (r) => {
  const raw = r.Progreso ?? r.progreso ?? 0
  const num = Number(raw)
  return {
    id: r.ID ?? r.id,
    carpetaId: r.CarpetaID ?? r.carpeta_id ?? r.carpetaId ?? 0,
    url: r.URL ?? r.url ?? '',
    titulo: r.Titulo ?? r.title ?? '',
    imagen: r.Imagen ?? r.image ?? '',
    descripcion: r.Descripcion ?? r.description ?? '',
    tipo: String(r.Tipo ?? r.tipo ?? r.type ?? 'otro').toLowerCase(),
    estado: r.Estado ?? r.estado ?? r.status ?? 'pendiente',
    progreso: Number.isFinite(num) ? Math.min(100, Math.max(0, Math.round(num))) : 0,
  }
}

const normStats = (s) => ({
  total: s?.Total ?? s?.total ?? 0,
  completados: s?.Completados ?? s?.completados ?? 0,
  pendientes: s?.Pendientes ?? s?.pendientes ?? 0,
  enCurso: s?.EnCurso ?? s?.en_curso ?? s?.enCurso ?? 0,
  promedio: s?.Promedio ?? s?.promedio ?? 0,
})

// Backend estados -> card presentation (bar width always comes from progreso real)
const CARD_STATUS = {
  done: { label: 'Completado', color: '#5FA97B' },
  progress: { label: 'En curso', color: '#C8C8C8' },
  pending: { label: 'Pendiente', color: '#C9964A' },
}
const toCardStatus = (estado) =>
  estado === 'completado' ? 'done' : estado === 'en_curso' ? 'progress' : 'pending'
const SIGUIENTE_ESTADO = { pendiente: 'en_curso', en_curso: 'completado', completado: 'pendiente' }

const TIPO_LABEL = { youtube: 'YOUTUBE', articulo: 'ARTÍCULO', otro: 'OTRO' }
const thumbVariant = (id) => `thumb-v${(Number(id) || 0) % 6 + 1}`

function hostDe(url) {
  try {
    return new URL(url).hostname.replace(/^www\./, '')
  } catch {
    return ''
  }
}

/* ───────── ICONS (Lucide stroke) ───────── */
const Logo = ({ size = 22 }) => (
  <svg width={size} height={size} viewBox="0 0 100 100" fill="none" aria-hidden="true">
    <path d="M24 50 C24 36 36 26 50 26 C64 26 76 36 76 50" stroke="#C8C8C8" strokeWidth="5" fill="none" strokeLinecap="round" />
    <path d="M24 50 C24 64 36 74 50 74 C64 74 76 64 76 50" stroke="#C8C8C8" strokeWidth="5" fill="none" strokeLinecap="round" />
  </svg>
)

const Icon = ({ name, size = 14 }) => {
  const icons = {
    book: (<><path d="M4 19.5A2.5 2.5 0 0 1 6.5 17H20" /><path d="M6.5 2H20v20H6.5A2.5 2.5 0 0 1 4 19.5v-15A2.5 2.5 0 0 1 6.5 2z" /></>),
    folder: (<path d="M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z" />),
    clock: (<><circle cx="12" cy="12" r="10" /><polyline points="12 6 12 12 16 14" /></>),
    check: (<><path d="M22 11.08V12a10 10 0 1 1-5.93-9.14" /><polyline points="22 4 12 14.01 9 11.01" /></>),
    download: (<><path d="M12 3v12" /><path d="M7 10l5 5 5-5" /><path d="M5 21h14" /></>),
    settings: (<><circle cx="12" cy="12" r="3" /><path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 1 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 1 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 1 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 1 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z" /></>),
    search: (<><circle cx="11" cy="11" r="8" /><path d="M21 21l-4.35-4.35" /></>),
    plus: (<><line x1="12" y1="5" x2="12" y2="19" /><line x1="5" y1="12" x2="19" y2="12" /></>),
    more: (<><circle cx="12" cy="12" r="1" /><circle cx="12" cy="5" r="1" /><circle cx="12" cy="19" r="1" /></>),
    play: (<path d="M8 5v14l11-7z" />),
    file: (<><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z" /><polyline points="14 2 14 8 20 8" /></>),
    trash: (<><polyline points="3 6 5 6 21 6" /><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2" /></>),
    x: (<><line x1="18" y1="6" x2="6" y2="18" /><line x1="6" y1="6" x2="18" y2="18" /></>),
    external: (<><path d="M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6" /><polyline points="15 3 21 3 21 9" /><line x1="10" y1="14" x2="21" y2="3" /></>),
  }
  return (
    <svg
      width={size} height={size} viewBox="0 0 24 24" aria-hidden="true"
      fill={name === 'play' ? 'currentColor' : 'none'}
      stroke={name === 'play' ? 'none' : 'currentColor'}
      strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round"
    >
      {icons[name]}
    </svg>
  )
}

/* ───────── SIDEBAR ───────── */
function Sidebar({ currentView, onChangeView, counts }) {
  const navGroups = [
    {
      label: 'Biblioteca',
      items: [
        { id: 'all', label: 'Todos', icon: 'book' },
        { id: 'dashboard', label: 'Dashboard', icon: 'play' },
        { id: 'folders', label: 'Carpetas', icon: 'folder' },
      ],
    },
    {
      label: 'Acciones',
      items: [
        { id: 'export', label: 'Exportar', icon: 'download' },
        { id: 'settings', label: 'Ajustes', icon: 'settings' },
      ],
    },
  ]
  return (
    <aside className="sidebar">
      <div className="brand">
        <Logo size={22} />
        <span className="brand-name">BYTHOS</span>
      </div>
      {navGroups.map((group) => (
        <div key={group.label} className="nav-group">
          <div className="nav-label">{group.label}</div>
          {group.items.map((item) => {
            const isActive = currentView === item.id
            const count = counts?.[item.id]
            const isAction = item.id === 'export' || item.id === 'settings'
            return (
              <button
                key={item.id}
                onClick={() => onChangeView(item.id)}
                className={`nav-item${isActive ? ' active' : ''}`}
              >
                <Icon name={item.icon} size={14} />
                <span>{item.label}</span>
                {count !== undefined && !isAction && (
                  <span className="nav-count">{count}</span>
                )}
              </button>
            )
          })}
        </div>
      ))}
      <div className="sidebar-foot">v1.0.0 · Local</div>
    </aside>
  )
}

/* ───────── TOPBAR ───────── */
function Topbar({ query, onQueryChange, onAdd }) {
  return (
    <div className="topbar">
      <div className="search">
        <Icon name="search" size={13} />
        <input
          type="text"
          value={query}
          onChange={(e) => onQueryChange(e.target.value)}
          placeholder="Buscar recursos..."
        />
      </div>
      <div className="topbar-actions">
        <button className="btn-primary" onClick={onAdd}>
          <Icon name="plus" size={12} />
          Añadir link
        </button>
        <button className="icon-btn" title="Más opciones">
          <Icon name="more" size={13} />
        </button>
      </div>
    </div>
  )
}

/* ───────── PAGE HEADER ───────── */
function PageHeader({ title, subtitle, filter, onFilterChange, showFilter = true }) {
  const filters = [
    { id: 'all', label: 'Todos' },
    { id: 'youtube', label: 'YouTube' },
    { id: 'article', label: 'Artículos' },
  ]
  return (
    <div className="page-header">
      <div>
        <h1 className="page-title">{title}</h1>
        <p className="page-subtitle">{subtitle}</p>
      </div>
      {showFilter && (
        <div className="filter-row">
          {filters.map((f) => (
            <button
              key={f.id}
              onClick={() => onFilterChange(f.id)}
              className={`filter-pill${filter === f.id ? ' active' : ''}`}
            >
              {f.label}
            </button>
          ))}
        </div>
      )}
    </div>
  )
}

/* ───────── PROGRESS ───────── */
function ProgressCard({ dotColor, label, num, fillColor, fillWidth }) {
  return (
    <div className="progress-card">
      <div className="progress-label">
        <span className="progress-dot" style={{ background: dotColor }} />
        {label}
      </div>
      <div className="progress-num">{num}</div>
      <div className="bar">
        <div style={{ background: fillColor, width: fillWidth }} />
      </div>
    </div>
  )
}

function ProgressOverview({ stats }) {
  const { completados, enCurso, pendientes, total, promedio } = stats
  return (
    <div>
      <div className="progress-grid">
        <ProgressCard dotColor="#5FA97B" label="Completados" num={completados} fillColor="#5FA97B" fillWidth={total ? `${(completados / total) * 100}%` : '0%'} />
        <ProgressCard dotColor="#C8C8C8" label="En curso" num={enCurso} fillColor="#C8C8C8" fillWidth={total ? `${(enCurso / total) * 100}%` : '0%'} />
        <ProgressCard dotColor="#C9964A" label="Pendientes" num={pendientes} fillColor="#C9964A" fillWidth={total ? `${(pendientes / total) * 100}%` : '0%'} />
      </div>
      <p className="progress-avg">Promedio total: {promedio ?? 0}% en {total} elementos</p>
    </div>
  )
}

/* ───────── RESOURCE CARD ───────── */
// No duration/category in backend: secondary line shows Descripcion
// (truncated), falling back to folder + host context.
function ResourceCard({ resource, folderName, onCycleStatus, onDelete, onOpen }) {
  const [hovered, setHovered] = useState(false)
  const status = CARD_STATUS[toCardStatus(resource.estado)]
  const pct = Number.isFinite(Number(resource.progreso)) ? Math.min(100, Math.max(0, Math.round(Number(resource.progreso)))) : 0
  const isYouTube = resource.tipo === 'youtube'
  const secondary = resource.descripcion
    || [folderName, hostDe(resource.url)].filter(Boolean).join(' · ')
    || 'Sin descripción'

  return (
    <div
      className="res-card"
      onMouseEnter={() => setHovered(true)}
      onMouseLeave={() => setHovered(false)}
    >
      <div className={`thumb ${thumbVariant(resource.id)}`}>
        {resource.imagen && (
          <img
            src={resource.imagen}
            alt=""
            loading="lazy"
            onError={(e) => { e.currentTarget.style.display = 'none' }}
          />
        )}
        <div className="thumb-badge">
          <Icon name={isYouTube ? 'play' : 'file'} size={8} />
          {TIPO_LABEL[resource.tipo] ?? 'OTRO'}
        </div>
        {hovered && (
          <div className="thumb-actions animate-fade-in">
            <button className="thumb-btn" title="Abrir" onClick={(e) => { e.stopPropagation(); onOpen(resource) }}>
              <Icon name="external" size={10} />
            </button>
            <button className="thumb-btn danger" title="Eliminar" onClick={(e) => { e.stopPropagation(); onDelete(resource.id) }}>
              <Icon name="trash" size={10} />
            </button>
          </div>
        )}
      </div>
      <div className="res-body">
        <div className="res-title line-clamp-2" title={resource.titulo || resource.url}>
          {resource.titulo || resource.url}
        </div>
        <div className="res-meta line-clamp-2" title={secondary}>{secondary}</div>
        <div className="res-foot">
          <div className="bar">
            <div style={{ background: status.color, width: `${pct}%` }} />
          </div>
          <span className="pct-label">{pct}%</span>
          <button
            className="status-btn"
            style={{ color: status.color }}
            title="Cambiar estado"
            onClick={(e) => { e.stopPropagation(); onCycleStatus(resource.id) }}
          >
            {status.label}
          </button>
        </div>
      </div>
    </div>
  )
}

/* ───────── RESOURCE GRID ───────── */
function ResourceGrid({ resources, folderById, onCycleStatus, onDelete, onOpen }) {
  if (resources.length === 0) {
    return (
      <div className="empty">
        <div className="empty-icon">
          <Icon name="search" size={20} />
        </div>
        <p className="empty-title">No se encontraron recursos</p>
        <p className="empty-sub">Prueba con otro término o cambia el filtro</p>
      </div>
    )
  }
  return (
    <div className="res-grid">
      {resources.map((r) => (
        <ResourceCard
          key={r.id}
          resource={r}
          folderName={folderById[r.carpetaId] ?? ''}
          onCycleStatus={onCycleStatus}
          onDelete={onDelete}
          onOpen={onOpen}
        />
      ))}
    </div>
  )
}

/* ───────── ADD MODAL ───────── */
function AddResourceModal({ open, onClose, onAdd, carpetas }) {
  const [url, setUrl] = useState('')
  const [titulo, setTitulo] = useState('')
  const [carpetaId, setCarpetaId] = useState('')

  useEffect(() => {
    if (open && carpetas.length > 0 && !carpetaId) setCarpetaId(String(carpetas[0].id))
    if (open && carpetas.length === 0) setCarpetaId('')
  }, [open, carpetas]) // eslint-disable-line react-hooks/exhaustive-deps

  if (!open) return null

  const handleSubmit = (e) => {
    e.preventDefault()
    if (!url.trim() || !carpetaId) return
    onAdd({ url: url.trim(), titulo: titulo.trim(), carpetaId: Number(carpetaId) })
    setUrl('')
    setTitulo('')
    onClose()
  }

  return (
    <div className="modal-overlay animate-fade-in" onClick={onClose}>
      <form className="modal animate-slide-up" onClick={(e) => e.stopPropagation()} onSubmit={handleSubmit}>
        <div className="modal-head">
          <h2 className="modal-title">Añadir recurso</h2>
          <button type="button" className="modal-close" onClick={onClose}>
            <Icon name="x" size={14} />
          </button>
        </div>
        <div className="field">
          <label className="field-label">URL del recurso *</label>
          <input
            type="url"
            autoFocus
            value={url}
            onChange={(e) => setUrl(e.target.value)}
            placeholder="https://youtube.com/watch?v=..."
            required
          />
        </div>
        <div className="field">
          <label className="field-label">Título (opcional)</label>
          <input
            type="text"
            value={titulo}
            onChange={(e) => setTitulo(e.target.value)}
            placeholder="Se detectará automáticamente"
          />
        </div>
        <div className="field">
          <label className="field-label">Carpeta *</label>
          {carpetas.length === 0 ? (
            <p className="field-hint">Crea primero una carpeta en la vista Carpetas.</p>
          ) : (
            <select value={carpetaId} onChange={(e) => setCarpetaId(e.target.value)} required>
              {carpetas.map((c) => (
                <option key={c.id} value={c.id}>{c.nombre}</option>
              ))}
            </select>
          )}
        </div>
        <div className="modal-actions">
          <button type="button" className="btn-ghost" onClick={onClose}>Cancelar</button>
          <button type="submit" className="btn-solid" disabled={!url.trim() || !carpetaId}>
            Añadir recurso
          </button>
        </div>
      </form>
    </div>
  )
}

/* ───────── FOLDERS VIEW ───────── */
// Local filter by folder progress (default Todas):
// completada = promedio 100, en curso = promedio 1-99.
// Fresh/empty folders (promedio 0 or no data) only show in Todas,
// mirroring resource states pendiente (0) / en_curso (1-99) / completado (100).
function FoldersView({ carpetas, progresos, onOpenFolder, onBorrarCarpeta, nuevaCarpeta, onNuevaCarpetaChange, onCrearCarpeta }) {
  const [filtro, setFiltro] = useState('todas')
  const promedioDe = (c) => {
    const p = progresos[c.id]
    if (!p || !p.total) return 0
    return Math.min(100, Math.max(0, Math.round(p.promedio ?? 0)))
  }
  const filtradas = carpetas.filter((c) => {
    if (filtro === 'completadas') return promedioDe(c) >= 100
    if (filtro === 'en_curso') {
      const prom = promedioDe(c)
      return prom > 0 && prom < 100
    }
    return true
  })
  const filtros = [
    { id: 'todas', label: 'Todas' },
    { id: 'en_curso', label: 'En curso' },
    { id: 'completadas', label: 'Completadas' },
  ]
  return (
    <div>
      <div className="panel">
        <p className="panel-title">Nueva carpeta</p>
        <p className="panel-sub">Agrupa tus recursos por tema: Go Backend, React…</p>
        <form className="inline-form" onSubmit={onCrearCarpeta}>
          <input
            value={nuevaCarpeta}
            onChange={(e) => onNuevaCarpetaChange(e.target.value)}
            placeholder="Nombre de la carpeta…"
          />
          <button type="submit" className="btn-outline" disabled={!nuevaCarpeta.trim()}>+ Crear</button>
        </form>
      </div>
      <div className="filter-row" style={{ marginBottom: 12 }}>
        {filtros.map((f) => (
          <button
            key={f.id}
            onClick={() => setFiltro(f.id)}
            className={`filter-pill${filtro === f.id ? ' active' : ''}`}
          >
            {f.label}
          </button>
        ))}
      </div>
      {filtradas.length === 0 ? (
        <div className="empty">
          <div className="empty-icon"><Icon name="folder" size={20} /></div>
          <p className="empty-title">{carpetas.length === 0 ? 'Sin carpetas todavía' : 'Sin carpetas en este filtro'}</p>
          <p className="empty-sub">{carpetas.length === 0 ? 'Crea la primera arriba para empezar a guardar' : 'Cambia a Todas para ver el resto'}</p>
        </div>
      ) : (
        <div className="folder-list">
          {filtradas.map((c) => {
            const p = progresos[c.id]
            const pct = p ? (p.total ? Math.round((p.completados / p.total) * 100) : 0) : 0
            const prom = p?.promedio ?? 0
            return (
              <div key={c.id} className="folder-card">
                <div className="folder-icon"><Icon name="folder" size={16} /></div>
                <div className="folder-info">
                  <p className="folder-name">{c.nombre}</p>
                  <div className="bar"><div style={{ background: '#5FA97B', width: `${prom}%` }} /></div>
                  <p className="folder-stats">
                    {p
                      ? `${p.total} elementos · ${p.completados} completados · Promedio ${prom}%`
                      : 'Calculando progreso…'}
                  </p>
                </div>
                <button className="btn-outline" onClick={() => onOpenFolder(c.id)}>Ver recursos</button>
                <button
                  className="icon-btn"
                  title={`Eliminar carpeta ${c.nombre}`}
                  onClick={() => onBorrarCarpeta(c.id, c.nombre)}
                >
                  <Icon name="trash" size={13} />
                </button>
              </div>
            )
          })}
        </div>
      )}
    </div>
  )
}

/* ───────── DASHBOARD VIEW ───────── */
// Reuses stats + progresos already loaded (no new endpoints):
// global average on top, per-folder table, AI flow hint at the bottom.
function DashboardView({ stats, carpetas, progresos, onGoExport }) {
  const promedio = Math.min(100, Math.max(0, Math.round(stats.promedio ?? 0)))
  return (
    <div>
      <div className="panel dash-hero">
        <div className="dash-hero-top">
          <div>
            <p className="panel-title">Promedio global</p>
            <p className="panel-sub">
              {stats.total} elementos · {stats.completados} completados · {stats.enCurso} en curso · {stats.pendientes} pendientes
            </p>
          </div>
          <p className="dash-avg">{promedio}%</p>
        </div>
        <div className="bar"><div style={{ background: '#5FA97B', width: `${promedio}%` }} /></div>
      </div>
      <div className="panel">
        <p className="panel-title">Por carpeta</p>
        {carpetas.length === 0 ? (
          <p className="panel-sub">Crea tu primera carpeta en la vista Carpetas.</p>
        ) : (
          <div className="dash-table-wrap">
            <table className="dash-table">
              <thead>
                <tr>
                  <th>Carpeta</th>
                  <th>Total</th>
                  <th>Completados</th>
                  <th>En curso</th>
                  <th>Pendientes</th>
                  <th>Promedio</th>
                </tr>
              </thead>
              <tbody>
                {carpetas.map((c) => {
                  const p = progresos[c.id]
                  const prom = p ? Math.min(100, Math.max(0, Math.round(p.promedio ?? 0))) : 0
                  return (
                    <tr key={c.id}>
                      <td>{c.nombre}</td>
                      <td>{p ? p.total : '…'}</td>
                      <td>{p ? p.completados : '…'}</td>
                      <td>{p ? p.enCurso : '…'}</td>
                      <td>{p ? p.pendientes : '…'}</td>
                      <td>
                        <div className="dash-bar-row">
                          <div className="bar dash-bar">
                            <div style={{ background: '#5FA97B', width: `${prom}%` }} />
                          </div>
                          <span className="dash-pct">{p ? `${prom}%` : '…'}</span>
                        </div>
                      </td>
                    </tr>
                  )
                })}
              </tbody>
            </table>
          </div>
        )}
      </div>
      <div className="panel">
        <p className="panel-title">Flujo IA</p>
        <p className="panel-sub">Exportar → repasar en tu IA favorita → Importar avance.</p>
        <button className="btn-outline" onClick={onGoExport}>Ir a Exportar</button>
      </div>
    </div>
  )
}

/* ───────── EXPORT VIEW ───────── */
async function copiarTexto(texto) {
  try {
    await navigator.clipboard.writeText(texto)
    return true
  } catch {
    try {
      const ta = document.createElement('textarea')
      ta.value = texto
      document.body.appendChild(ta)
      ta.select()
      document.execCommand('copy')
      ta.remove()
      return true
    } catch {
      return false
    }
  }
}

function ExportView({ carpetas, carpetaId, onCarpetaChange, formato, onExportar, cargando, exportado, copiado, onCopiar, importTexto, onImportTextoChange, onImportar, importando, importResultado }) {
  return (
    <div>
      <div className="panel">
        <p className="panel-title">Carpeta a exportar</p>
        <p className="panel-sub">El texto se genera en tu PC; nada sale solo. Copia el bloque en tu IA y pídele que devuelva solo líneas - [N%] Título con su marca intacta (N de 0 a 100, una por línea, sin texto extra).</p>
        {carpetas.length === 0 ? (
          <p className="panel-sub">Crea primero una carpeta para exportar.</p>
        ) : (
          <div className="inline-form">
            <select
              value={carpetaId}
              onChange={(e) => onCarpetaChange(e.target.value)}
              style={{ flex: 1, padding: '8px 12px', background: '#0A0A0A', border: '1px solid #1F1F1F', borderRadius: 6, fontSize: 12, color: '#EDEDED', fontFamily: 'inherit' }}
            >
              {carpetas.map((c) => (
                <option key={c.id} value={c.id}>{c.nombre}</option>
              ))}
            </select>
          </div>
        )}
        <div className="export-grid">
          {[
            { id: 'markdown', label: 'Markdown' },
            { id: 'gemini', label: 'Gemini' },
            { id: 'notebooklm', label: 'NotebookLM' },
          ].map((f) => (
            <button
              key={f.id}
              className="btn-outline"
              disabled={!carpetaId || cargando}
              onClick={() => onExportar(f.id)}
            >
              {f.label}
            </button>
          ))}
          <button
            className="btn-outline"
            disabled={!carpetaId}
            onClick={() => window.open(`/api/carpetas/${carpetaId}/export?format=drive`, '_blank')}
            title="Descarga el .md para subirlo a Drive"
          >
            Google Drive
          </button>
        </div>
        {cargando && <p className="panel-sub">Generando…</p>}
        {formato && exportado && !cargando && (
          <div className="export-out">
            <button className="btn-outline" onClick={onCopiar}>
              {copiado ? 'Copiado' : 'Copiar'}
            </button>
            <pre className="export-pre">{exportado}</pre>
          </div>
        )}
      </div>
      <div className="panel">
        <p className="panel-title">Importar avance</p>
        <p className="panel-sub">Pega la respuesta de tu IA con líneas - [N%] Título intactas (N de 0 a 100, una por línea, conserva cada marca id) y guardamos el % de cada link.</p>
        <textarea
          className="import-area"
          value={importTexto}
          onChange={(e) => onImportTextoChange(e.target.value)}
          placeholder="- [40%] Intro React <!-- id:7 -->"
          rows={6}
          disabled={!carpetaId || importando}
        />
        <div className="export-grid">
          <button
            className="btn-outline"
            disabled={!carpetaId || !importTexto.trim() || importando}
            onClick={onImportar}
          >
            {importando ? 'Importando…' : 'Importar'}
          </button>
        </div>
        {importResultado && (
          <p className="panel-sub">
            {importResultado.actualizados} actualizados · {importResultado.omitidos} omitidos
          </p>
        )}
      </div>
    </div>
  )
}

/* ───────── SETTINGS VIEW ───────── */
function SettingsView({ salud, stats, totalCarpetas }) {
  return (
    <div className="panel">
      <div className="setting-row">
        <span className="setting-key">Backend</span>
        <span className="setting-val">
          <span className={`status-dot ${salud ? 'on' : 'off'}`} />
          {salud === null ? 'Comprobando…' : salud ? 'En línea' : 'Sin conexión'}
        </span>
      </div>
      <div className="setting-row">
        <span className="setting-key">Almacenamiento</span>
        <span className="setting-val">Local · bythos.db en tu PC</span>
      </div>
      <div className="setting-row">
        <span className="setting-key">Biblioteca</span>
        <span className="setting-val">
          {stats.total} recursos · {totalCarpetas} carpetas · {stats.completados} completados
        </span>
      </div>
      <div className="setting-row">
        <span className="setting-key">Tema</span>
        <span className="setting-val">Oscuro</span>
      </div>
      <div className="setting-row">
        <span className="setting-key">Versión</span>
        <span className="setting-val">v1.0.0 · Local</span>
      </div>
    </div>
  )
}

/* ───────── APP ───────── */
export default function App() {
  const [view, setView] = useState('all')
  const [typeFilter, setTypeFilter] = useState('all')
  const [query, setQuery] = useState('')
  const [modalOpen, setModalOpen] = useState(false)
  const [carpetaFiltro, setCarpetaFiltro] = useState('')

  const [carpetas, setCarpetas] = useState([])
  const [recursos, setRecursos] = useState([])
  const [stats, setStats] = useState({ total: 0, completados: 0, pendientes: 0, enCurso: 0, promedio: 0 })
  const [progresos, setProgresos] = useState({})
  const [cargando, setCargando] = useState(true)
  const [error, setError] = useState('')

  const [nuevaCarpeta, setNuevaCarpeta] = useState('')
  const [exportCarpetaId, setExportCarpetaId] = useState('')
  const [exportFormato, setExportFormato] = useState('')
  const [exportado, setExportado] = useState('')
  const [exportCargando, setExportCargando] = useState(false)
  const [copiado, setCopiado] = useState(false)
  const [importTexto, setImportTexto] = useState('')
  const [importando, setImportando] = useState(false)
  const [importResultado, setImportResultado] = useState(null)
  const [salud, setSalud] = useState(null)

  async function recargar() {
    try {
      setError('')
      const [cs, rs, st] = await Promise.all([
        api.listarCarpetas(),
        api.listarRecursos(0),
        api.stats(),
      ])
      const carpetasNorm = (Array.isArray(cs) ? cs : []).map(normCarpeta)
      setCarpetas(carpetasNorm)
      setRecursos((Array.isArray(rs) ? rs : []).map(normRecurso))
      setStats(normStats(st))
      setExportCarpetaId((prev) => {
        if (prev && carpetasNorm.some((c) => String(c.id) === String(prev))) return prev
        return carpetasNorm.length > 0 ? String(carpetasNorm[0].id) : ''
      })
    } catch (e) {
      setError(e.message)
    } finally {
      setCargando(false)
    }
  }

  useEffect(() => { recargar() }, [])

  // Folder progress bars: fetched when the folders or dashboard view is open
  // (both reuse the same progresos map, no new endpoints).
  useEffect(() => {
    if ((view !== 'folders' && view !== 'dashboard') || carpetas.length === 0) return
    let vivo = true
    Promise.all(
      carpetas.map((c) =>
        api.progresoCarpeta(c.id).then(normStats).catch(() => null),
      ),
    ).then((res) => {
      if (!vivo) return
      const mapa = {}
      carpetas.forEach((c, i) => { if (res[i]) mapa[c.id] = res[i] })
      setProgresos(mapa)
    })
    return () => { vivo = false }
  }, [view, carpetas])

  // Backend status for the settings view
  useEffect(() => {
    if (view !== 'settings') return
    api.salud().then(() => setSalud(true)).catch(() => setSalud(false))
  }, [view])

  const folderById = useMemo(() => {
    const mapa = {}
    carpetas.forEach((c) => { mapa[c.id] = c.nombre })
    return mapa
  }, [carpetas])

  const counts = useMemo(() => ({
    all: stats.total,
    folders: carpetas.length,
  }), [stats, carpetas])

  const visibles = useMemo(() => {
    let lista = recursos
    if (typeFilter === 'youtube') lista = lista.filter((r) => r.tipo === 'youtube')
    if (typeFilter === 'article') lista = lista.filter((r) => r.tipo === 'articulo')
    if (carpetaFiltro) lista = lista.filter((r) => String(r.carpetaId) === String(carpetaFiltro))
    if (query.trim()) {
      const q = query.trim().toLowerCase()
      lista = lista.filter((r) =>
        r.titulo.toLowerCase().includes(q)
        || r.url.toLowerCase().includes(q)
        || r.descripcion.toLowerCase().includes(q),
      )
    }
    return [...lista].sort((a, b) => Number(b.id) - Number(a.id))
  }, [recursos, view, typeFilter, carpetaFiltro, query])

  const pageTitle = {
    all: 'Todos los recursos', dashboard: 'Dashboard', folders: 'Carpetas',
    export: 'Exportar', settings: 'Ajustes',
  }[view] || 'Biblioteca'

  const pageSubtitle = (view === 'all')
    ? `${stats.total} elementos · ${stats.completados} completados · ${stats.enCurso} en curso · Promedio ${stats.promedio ?? 0}%`
    : view === 'dashboard'
      ? `${stats.total} elementos · Promedio ${stats.promedio ?? 0}% en ${carpetas.length} carpetas`
      : view === 'folders'
      ? `${carpetas.length} carpetas activas`
      : view === 'export'
        ? 'Genera textos para repasar en tu IA favorita'
        : 'Estado de la aplicación y datos locales'

  function changeView(v) {
    setView(v)
    if (v === 'all') setCarpetaFiltro('')
    if (v !== 'export') { setExportado(''); setExportFormato(''); setCopiado(false); setImportTexto(''); setImportResultado(null) }
  }

  async function handleCycleStatus(id) {
    const actual = recursos.find((r) => String(r.id) === String(id))
    if (!actual) return
    try {
      await api.cambiarEstado(id, SIGUIENTE_ESTADO[actual.estado] ?? 'pendiente')
      recargar()
    } catch (e) {
      setError(e.message)
    }
  }

  async function handleDelete(id) {
    try {
      await api.borrarRecurso(id)
      recargar()
    } catch (e) {
      setError(e.message)
    }
  }

  function handleOpen(resource) {
    if (resource.url) window.open(resource.url, '_blank', 'noopener')
  }

  async function handleAdd({ url, titulo, carpetaId }) {
    try {
      await api.guardarRecurso(url, carpetaId, titulo ? { titulo } : {})
      recargar()
    } catch (e) {
      setError(e.message)
    }
  }

  async function handleCrearCarpeta(e) {
    e.preventDefault()
    if (!nuevaCarpeta.trim()) return
    try {
      await api.crearCarpeta(nuevaCarpeta.trim())
      setNuevaCarpeta('')
      recargar()
    } catch (e) {
      setError(e.message)
    }
  }

  async function handleBorrarCarpeta(id, nombre) {
    const etiqueta = nombre ? `"${nombre}"` : 'esta carpeta'
    if (!window.confirm(`¿Eliminar la carpeta ${etiqueta} y todos sus recursos? Esta acción no se puede deshacer.`)) return
    try {
      await api.borrarCarpeta(id)
      recargar()
    } catch (e) {
      setError(e.message || 'No se pudo borrar la carpeta')
    }
  }

  async function handleExportar(formato) {
    if (!exportCarpetaId) return
    try {
      setExportCargando(true)
      setCopiado(false)
      setExportado(await api.exportar(exportCarpetaId, formato))
      setExportFormato(formato)
    } catch (e) {
      setError(e.message)
    } finally {
      setExportCargando(false)
    }
  }

  async function handleCopiar() {
    if (await copiarTexto(exportado)) setCopiado(true)
  }

  async function handleImportar() {
    if (!exportCarpetaId || !importTexto.trim()) return
    try {
      setImportando(true)
      setError('')
      const res = await api.importarAvance(exportCarpetaId, importTexto)
      setImportResultado({
        actualizados: res?.actualizados ?? res?.Actualizados ?? 0,
        omitidos: res?.omitidos ?? res?.Omitidos ?? 0,
      })
      setImportTexto('')
      recargar()
    } catch (e) {
      setError(e.message)
    } finally {
      setImportando(false)
    }
  }

  const esVistaRecursos = view === 'all'

  return (
    <div className="app">
      <Sidebar currentView={view} onChangeView={changeView} counts={counts} />
      <main className="main">
        <Topbar query={query} onQueryChange={setQuery} onAdd={() => setModalOpen(true)} />
        <div className="content">
          {esVistaRecursos ? (
            <PageHeader
              title={pageTitle}
              subtitle={pageSubtitle}
              filter={typeFilter}
              onFilterChange={setTypeFilter}
            />
          ) : (
            <PageHeader title={pageTitle} subtitle={pageSubtitle} showFilter={false} />
          )}

          {error && (
            <div className="error-banner">
              <span>{error}</span>
              <button onClick={recargar}>Reintentar</button>
            </div>
          )}

          {cargando ? (
            <div className="empty">
              <p className="empty-title">Cargando biblioteca…</p>
            </div>
          ) : esVistaRecursos ? (
            <>
              <ProgressOverview stats={stats} />
              {carpetaFiltro && folderById[carpetaFiltro] && (
                <div>
                  <span className="folder-chip">
                    {folderById[carpetaFiltro]}
                    <button onClick={() => setCarpetaFiltro('')}>Quitar</button>
                  </span>
                </div>
              )}
              <ResourceGrid
                resources={visibles}
                folderById={folderById}
                onCycleStatus={handleCycleStatus}
                onDelete={handleDelete}
                onOpen={handleOpen}
              />
            </>
          ) : view === 'dashboard' ? (
            <DashboardView
              stats={stats}
              carpetas={carpetas}
              progresos={progresos}
              onGoExport={() => setView('export')}
            />
          ) : view === 'folders' ? (
            <FoldersView
              carpetas={carpetas}
              progresos={progresos}
              onOpenFolder={(id) => { setCarpetaFiltro(String(id)); setView('all') }}
              onBorrarCarpeta={handleBorrarCarpeta}
              nuevaCarpeta={nuevaCarpeta}
              onNuevaCarpetaChange={setNuevaCarpeta}
              onCrearCarpeta={handleCrearCarpeta}
            />
          ) : view === 'export' ? (
            <ExportView
              carpetas={carpetas}
              carpetaId={exportCarpetaId}
              onCarpetaChange={(v) => { setExportCarpetaId(v); setImportResultado(null) }}
              formato={exportFormato}
              onExportar={handleExportar}
              cargando={exportCargando}
              exportado={exportado}
              copiado={copiado}
              onCopiar={handleCopiar}
              importTexto={importTexto}
              onImportTextoChange={setImportTexto}
              onImportar={handleImportar}
              importando={importando}
              importResultado={importResultado}
            />
          ) : (
            <SettingsView salud={salud} stats={stats} totalCarpetas={carpetas.length} />
          )}
        </div>
      </main>
      <AddResourceModal
        open={modalOpen}
        onClose={() => setModalOpen(false)}
        onAdd={handleAdd}
        carpetas={carpetas}
      />
    </div>
  )
}
