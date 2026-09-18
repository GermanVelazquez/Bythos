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

// Go structs have no `json:` tags, so agenda keys also arrive capitalized.
// CarpetaID llega null cuando la nota es suelta (sin carpeta).
const normAgenda = (a) => ({
  id: a.ID ?? a.id,
  fecha: a.Fecha ?? a.fecha ?? '',
  horaInicio: a.HoraInicio ?? a.hora_inicio ?? a.horaInicio ?? '',
  horaFin: a.HoraFin ?? a.hora_fin ?? a.horaFin ?? '',
  texto: a.Texto ?? a.texto ?? '',
  carpetaId: a.CarpetaID ?? a.carpeta_id ?? a.carpetaId ?? null,
  carpetaNombre: a.CarpetaNombre ?? a.carpeta_nombre ?? a.carpetaNombre ?? '',
})

const normActividad = (x) => ({
  fecha: x.Fecha ?? x.fecha ?? '',
  total: x?.Total ?? x?.total ?? 0,
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
      <div className="sidebar-foot">v1.0.2 · Local</div>
    </aside>
  )
}

/* ───────── TOPBAR ───────── */
// showSearch se apaga en la vista Todos: ahí no hay lista que filtrar,
// solo calendario. Default true para no tocar las demás vistas.
function Topbar({ query, onQueryChange, onAdd, showSearch = true }) {
  return (
    <div className="topbar">
      {showSearch ? (
        <div className="search">
          <Icon name="search" size={13} />
          <input
            type="text"
            value={query}
            onChange={(e) => onQueryChange(e.target.value)}
            placeholder="Buscar recursos..."
          />
        </div>
      ) : (
        <div style={{ flex: 1 }} />
      )}
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
        <a
          className="res-title line-clamp-2"
          title={`${resource.titulo || resource.url} — Abrir`}
          href={resource.url || '#'}
          target="_blank"
          rel="noopener noreferrer"
          onClick={(e) => { if (!resource.url) e.preventDefault(); e.stopPropagation() }}
          style={{ display: 'block', cursor: resource.url ? 'pointer' : 'default', textDecoration: 'none', color: 'inherit' }}
        >
          {resource.titulo || resource.url}
        </a>
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
function FoldersView({ carpetas, progresos, recursos, folderById, onCycleStatus, onDelete, onOpen, onBorrarCarpeta, nuevaCarpeta, onNuevaCarpetaChange, onCrearCarpeta }) {
  const [filtro, setFiltro] = useState('todas')
  const [abiertaId, setAbiertaId] = useState('')
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
  // Carpeta abierta: se muestra DENTRO de Carpetas (no navega a Todos).
  const abierta = abiertaId ? carpetas.find((c) => String(c.id) === String(abiertaId)) : null
  if (abierta) {
    const p = progresos[abierta.id]
    const prom = p && p.total ? Math.min(100, Math.max(0, Math.round(p.promedio ?? 0))) : 0
    const recs = (recursos || [])
      .filter((r) => String(r.carpetaId) === String(abierta.id))
      .sort((a, b) => Number(b.id) - Number(a.id))
    return (
      <div>
        <button className="btn-outline" style={{ marginBottom: 12 }} onClick={() => setAbiertaId('')}>
          ← Volver a carpetas
        </button>
        <div className="panel">
          <p className="panel-title">{abierta.nombre}</p>
          {p ? (
            <>
              <div className="progress-grid">
                <ProgressCard dotColor="#5FA97B" label="Completados" num={p.completados} fillColor="#5FA97B" fillWidth={p.total ? `${(p.completados / p.total) * 100}%` : '0%'} />
                <ProgressCard dotColor="#C8C8C8" label="En curso" num={p.enCurso} fillColor="#C8C8C8" fillWidth={p.total ? `${(p.enCurso / p.total) * 100}%` : '0%'} />
                <ProgressCard dotColor="#C9964A" label="Pendientes" num={p.pendientes} fillColor="#C9964A" fillWidth={p.total ? `${(p.pendientes / p.total) * 100}%` : '0%'} />
              </div>
              <p className="progress-avg">Promedio carpeta: {prom}% en {p.total} elementos</p>
            </>
          ) : (
            <p className="panel-sub">Calculando progreso…</p>
          )}
        </div>
        <ResourceGrid
          resources={recs}
          folderById={folderById}
          onCycleStatus={onCycleStatus}
          onDelete={onDelete}
          onOpen={onOpen}
        />
      </div>
    )
  }
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
                <button className="btn-outline" onClick={() => setAbiertaId(String(c.id))}>Ver recursos</button>
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

/* ───────── CHARTS (SVG puro, sin dependencias) ───────── */
// Torta: porción del avance total que aporta cada elemento.
// Ojiva: promedio acumulado elemento por elemento (sube/baja al avanzar,
// sumar o quitar elementos). Ambos se recalculan solos en cada render.
const CHART_COLORS = ['#5FA97B', '#8FA8D8', '#C9964A', '#D88FB0', '#7FD4D8', '#A58FD8', '#D8C58F', '#E07A5F', '#81B29A', '#F2CC8F']

function PieChart({ items }) {
  const total = items.reduce((acc, r) => acc + (Number(r.progreso) || 0), 0)
  if (items.length === 0) return <p className="panel-sub">Esta carpeta no tiene elementos todavía.</p>
  const avg = Math.round(total / items.length)
  if (total <= 0) {
    return (
      <div>
        <svg viewBox="0 0 200 200" role="img" aria-label="Sin avance todavía" style={{ width: '100%', maxWidth: 240, display: 'block' }}>
          <circle cx="100" cy="100" r="70" fill="none" stroke="#2A2A2A" strokeWidth="28" />
          <text x="100" y="100" textAnchor="middle" dominantBaseline="central" fill="#8A8A8A" fontSize="22" fontWeight="700">0%</text>
        </svg>
        <p className="panel-sub">Sin avance todavía: marcá progreso en un elemento y la torta se dibuja sola.</p>
      </div>
    )
  }
  const R = 70
  const C = 2 * Math.PI * R
  let acc = 0
  const slices = items.map((it, i) => {
    const frac = (Number(it.progreso) || 0) / total
    const s = { it, i, frac, off: acc }
    acc += frac * C
    return s
  })
  return (
    <div>
      <svg viewBox="0 0 200 200" role="img" aria-label="Proporción de avance por elemento" style={{ width: '100%', maxWidth: 240, display: 'block' }}>
        {slices.filter((s) => s.frac > 0).map((s) => (
          <circle
            key={s.it.id ?? s.i}
            cx="100" cy="100" r={R} fill="none"
            stroke={CHART_COLORS[s.i % CHART_COLORS.length]}
            strokeWidth="28"
            strokeDasharray={`${Math.max(s.frac * C - 2, 1)} ${C}`}
            strokeDashoffset={C / 4 - s.off}
          >
            <title>{`${s.it.titulo || s.it.url}: ${s.it.progreso}%`}</title>
          </circle>
        ))}
        <text x="100" y="100" textAnchor="middle" dominantBaseline="central" fill="#EDEDED" fontSize="22" fontWeight="700">{avg}%</text>
      </svg>
      <div style={{ display: 'flex', flexDirection: 'column', gap: 6, marginTop: 12 }}>
        {items.map((it, i) => (
          <div key={it.id ?? i} style={{ display: 'flex', alignItems: 'center', gap: 8, fontSize: 12 }}>
            <span style={{ width: 10, height: 10, borderRadius: '50%', flexShrink: 0, background: CHART_COLORS[i % CHART_COLORS.length] }} />
            <span title={it.titulo || it.url} style={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', maxWidth: 220, color: '#C8C8C8' }}>
              {it.titulo || it.url}
            </span>
            <span style={{ marginLeft: 'auto', color: '#EDEDED', fontWeight: 600 }}>{it.progreso}%</span>
          </div>
        ))}
      </div>
    </div>
  )
}

function OjivaChart({ items }) {
  const W = 560, H = 260, PAD_L = 44, PAD_B = 32, PAD_T = 16, PAD_R = 20
  const sorted = [...items].sort((a, b) => Number(a.id) - Number(b.id))
  const n = sorted.length
  if (n === 0) return <p className="panel-sub">Esta carpeta no tiene elementos todavía.</p>
  const cum = []
  let acc = 0
  sorted.forEach((r, k) => {
    acc += Number(r.progreso) || 0
    cum.push(acc / (k + 1))
  })
  const X = (k) => (n === 1 ? PAD_L + (W - PAD_L - PAD_R) / 2 : PAD_L + (k * (W - PAD_L - PAD_R)) / (n - 1))
  const Y = (v) => PAD_T + (1 - v / 100) * (H - PAD_T - PAD_B)
  const line = cum.map((v, k) => `${X(k).toFixed(1)},${Y(v).toFixed(1)}`).join(' ')
  const area = `M ${X(0).toFixed(1)},${H - PAD_B} L ${line.replaceAll(' ', ' L ')} L ${X(n - 1).toFixed(1)},${H - PAD_B} Z`
  const step = Math.max(1, Math.ceil(n / 10))
  return (
    <svg viewBox={`0 0 ${W} ${H}`} role="img" aria-label="Ojiva de avance acumulado" style={{ width: '100%', display: 'block' }}>
      {[0, 25, 50, 75, 100].map((g) => (
        <g key={g}>
          <line x1={PAD_L} y1={Y(g)} x2={W - PAD_R} y2={Y(g)} stroke="#2A2A2A" strokeWidth="1" />
          <text x={PAD_L - 8} y={Y(g)} textAnchor="end" dominantBaseline="central" fill="#8A8A8A" fontSize="12">{g}%</text>
        </g>
      ))}
      <polygon points={area} fill="#5FA97B" opacity="0.15" />
      <polyline points={line} fill="none" stroke="#5FA97B" strokeWidth="2.5" strokeLinejoin="round" strokeLinecap="round" />
      {cum.map((v, k) => (
        <circle key={sorted[k].id ?? k} cx={X(k)} cy={Y(v)} r="4.5" fill="#101010" stroke="#5FA97B" strokeWidth="2.5">
          <title>{`Elemento ${k + 1} · ${sorted[k].titulo || sorted[k].url}: acumulado ${Math.round(v)}%`}</title>
        </circle>
      ))}
      {cum.map((v, k) => ((k % step === 0 || k === n - 1) ? (
        <text key={`x${k}`} x={X(k)} y={H - 10} textAnchor="middle" fill="#8A8A8A" fontSize="12">{k + 1}</text>
      ) : null))}
      <text x={W - PAD_R} y={H - 10} textAnchor="end" fill="#5A5A5A" fontSize="11">n = {n} elementos</text>
      <text x={X(n - 1)} y={Y(cum[n - 1]) - 12} textAnchor="end" fill="#EDEDED" fontSize="14" fontWeight="700">{Math.round(cum[n - 1])}%</text>
    </svg>
  )
}

/* ───────── DASHBOARD VIEW ───────── */
// Reuses stats + progresos already loaded (no new endpoints):
// global average on top, per-folder table, AI flow hint at the bottom.
function DashboardView({ stats, carpetas, progresos, recursos, onGoExport }) {
  const promedio = Math.min(100, Math.max(0, Math.round(stats.promedio ?? 0)))
  const [selId, setSelId] = useState('')
  const sel = carpetas.find((c) => String(c.id) === String(selId)) ?? carpetas[0] ?? null
  const itemsSel = sel ? (recursos || []).filter((r) => String(r.carpetaId) === String(sel.id)) : []
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
        <p className="panel-title">Detalle por carpeta</p>
        <p className="panel-sub">Elegí una carpeta para ver su torta de avance por elemento y su ojiva acumulada.</p>
        {carpetas.length === 0 ? (
          <p className="panel-sub">Crea tu primera carpeta en la vista Carpetas.</p>
        ) : (
          <div className="inline-form">
            <select
              value={sel ? String(sel.id) : ''}
              onChange={(e) => setSelId(e.target.value)}
              style={{ flex: 1, padding: '8px 12px', background: '#0A0A0A', border: '1px solid #1F1F1F', borderRadius: 6, fontSize: 12, color: '#EDEDED', fontFamily: 'inherit' }}
            >
              {carpetas.map((c) => (
                <option key={c.id} value={c.id}>{c.nombre}</option>
              ))}
            </select>
          </div>
        )}
      </div>
      {sel && (
        <div className="panel">
          <p className="panel-title">Gráficos — {sel.nombre}</p>
          <div style={{ display: 'flex', gap: 24, flexWrap: 'wrap', alignItems: 'flex-start' }}>
            <div style={{ flex: '1 1 260px', minWidth: 0 }}>
              <p className="panel-sub" style={{ fontWeight: 700, color: '#EDEDED' }}>Torta</p>
              <p className="panel-sub">Porción del avance total que aporta cada elemento.</p>
              <PieChart items={itemsSel} />
            </div>
            <div style={{ flex: '1 1 320px', minWidth: 0 }}>
              <p className="panel-sub" style={{ fontWeight: 700, color: '#EDEDED' }}>Ojiva</p>
              <p className="panel-sub">Promedio acumulado elemento por elemento: sube o baja cuando avanzás, sumás o quitás elementos.</p>
              <OjivaChart items={itemsSel} />
            </div>
          </div>
        </div>
      )}
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
        <span className="setting-val">v1.0.2 · Local</span>
      </div>
    </div>
  )
}

/* ───────── AGENDA (vista Todos) ───────── */
// La vista Todos es un calendario mensual + el detalle del día elegido.
// Semana lunes-domingo, 6 filas fijas (42 celdas) para que el alto no salte.
// Dos puntos por día: gris = historia (creaste algo ese día, /api/actividad),
// verde = tienes notas agendadas (/api/agenda). Ninguna dependencia nueva:
// solo los paneles y botones que ya usa el resto de la app.
const MESES = ['enero', 'febrero', 'marzo', 'abril', 'mayo', 'junio', 'julio', 'agosto', 'septiembre', 'octubre', 'noviembre', 'diciembre']
const DIAS_CORTO = ['L', 'M', 'M', 'J', 'V', 'S', 'D']
const DIAS_LARGO = ['domingo', 'lunes', 'martes', 'miércoles', 'jueves', 'viernes', 'sábado']

const pad2 = (n) => String(n).padStart(2, '0')
// Local a propósito: toISOString es UTC y te mueve de día cerca de la medianoche.
const isoLocal = (d) => `${d.getFullYear()}-${pad2(d.getMonth() + 1)}-${pad2(d.getDate())}`
const parseISO = (iso) => {
  const [y, m, d] = iso.split('-').map(Number)
  return new Date(y, m - 1, d)
}

// Ventana visible de 6 semanas para un mes: del lunes de la semana del día 1
// hasta 41 días después. Es el rango que se pide al backend (un solo fetch).
function ventanaDeMes(anio, mes) {
  const primero = new Date(anio, mes, 1)
  const desfase = (primero.getDay() + 6) % 7 // lunes=0 ... domingo=6
  const inicio = new Date(anio, mes, 1 - desfase)
  const celdas = []
  for (let i = 0; i < 42; i++) {
    celdas.push(new Date(inicio.getFullYear(), inicio.getMonth(), inicio.getDate() + i))
  }
  return { celdas, desde: isoLocal(celdas[0]), hasta: isoLocal(celdas[41]) }
}

function tituloDia(iso) {
  const d = parseISO(iso)
  return `${DIAS_LARGO[d.getDay()]} ${d.getDate()} de ${MESES[d.getMonth()]} de ${d.getFullYear()}`
}

function etiquetaHora(item) {
  if (item.horaInicio && item.horaFin) return `${item.horaInicio}–${item.horaFin}`
  if (item.horaInicio) return item.horaInicio
  return 'Sin hora'
}

// Espejo del backend (ver validarAgenda en db/agenda.go):
// avisa en el acto sin viaje a Go; Go igual revalida todo al guardar.
function validarNota({ texto, horaInicio, horaFin, carpetaId }) {
  if (!texto.trim() && !carpetaId) return 'Escribí una nota o elegí una carpeta'
  if (horaInicio && !/^\d{2}:\d{2}$/.test(horaInicio)) return 'Hora de inicio inválida. Usa HH:MM'
  if (horaFin && !/^\d{2}:\d{2}$/.test(horaFin)) return 'Hora de fin inválida. Usa HH:MM'
  if (horaFin && !horaInicio) return 'Si pones hora de fin, poné también la de inicio'
  if (horaInicio && horaFin && horaFin <= horaInicio) return 'La hora de fin debe ser posterior a la de inicio'
  return ''
}

const controlOscuro = {
  padding: '8px 12px', background: '#0A0A0A', border: '1px solid #1F1F1F',
  borderRadius: 6, fontSize: 12, color: '#EDEDED', fontFamily: 'inherit',
}

// Copy-paste grammar for an external AI: mirrors parseCompromisosMD
// (desktop/api/import_md.go). Frontend-only, no backend needed.
const PROMPT_IMPORT_MD = `Pasame mis compromisos con este formato exacto, sin texto fuera de los bloques:

## Compromiso: <título>
- Tipo: puntual | semanal
- Fecha: YYYY-MM-DD (solo puntual)
- Desde: YYYY-MM-DD (solo semanal)
- Hasta: YYYY-MM-DD (solo semanal)
- Horario: puntual usa HH-HH o HH:MM-HH:MM (ej: 10-12, 19:00-20:30, varios con coma). Semanal usa "<día> <rango>" separados por coma, días: lunes, martes, miércoles, jueves, viernes, sábado, domingo (ej: lunes 19-20, miércoles 19:00-20:30)
- Carpeta: <nombre> (opcional)
- Aula: <texto> (opcional)
- Docente: <nombre> (opcional)
- Modalidad: alias viejo, mejor no usar. anual necesita Desde (expande marzo-diciembre de ese año); 1er/2do cuatrimestre necesitan Desde y Hasta.

Ejemplo puntual:
## Compromiso: Parcial Física
- Tipo: puntual
- Fecha: 2026-04-10
- Horario: 10-12
- Aula: 101
- Docente: García

Ejemplo semanal:
## Compromiso: Física I
- Tipo: semanal
- Desde: 2026-03-02
- Hasta: 2026-06-15
- Horario: lunes 19-20, miércoles 19-20
- Carpeta: Física I`

function AgendaView({ carpetas }) {
  const ahora = new Date()
  const [mes, setMes] = useState({ anio: ahora.getFullYear(), mes: ahora.getMonth() })
  const [vista, setVista] = useState('mes')
  const [seleccionado, setSeleccionado] = useState(() => isoLocal(new Date()))
  const [diaAbierto, setDiaAbierto] = useState(null)
  const [items, setItems] = useState([])
  const [actividad, setActividad] = useState({})
  const [texto, setTexto] = useState('')
  const [horaInicio, setHoraInicio] = useState('')
  const [horaFin, setHoraFin] = useState('')
  const [carpetaId, setCarpetaId] = useState('')
  const [formError, setFormError] = useState('')
  const [importAbierto, setImportAbierto] = useState(false)
  const [importTexto, setImportTexto] = useState('')
  const [importNombre, setImportNombre] = useState('')
  const [importando, setImportando] = useState(false)
  const [importPreview, setImportPreview] = useState(null)
  const [importResultado, setImportResultado] = useState(null)
  const [importError, setImportError] = useState('')
  // Decisiones del menú por bloque (ver decisionesPayload): se arman en la
  // vista previa y viajan al confirmar. Se resetean con cada preview nuevo.
  const [importDecisiones, setImportDecisiones] = useState({})
  // Feedback for the copyable AI-prompt button (copiarTexto has fallback).
  const [promptCopiado, setPromptCopiado] = useState(false)
  // Lotes: el MD importado persiste como fuente editable/eliminable.
  const [lotes, setLotes] = useState([])
  const [lotesError, setLotesError] = useState('')
  const [editId, setEditId] = useState(null)
  const [editNombre, setEditNombre] = useState('')
  const [editTexto, setEditTexto] = useState('')
  const [editPreview, setEditPreview] = useState(null)
  const [editError, setEditError] = useState('')
  const [editDecisiones, setEditDecisiones] = useState({})
  const [loteBusy, setLoteBusy] = useState(false)
  // Overlay for loose blocks without an explicit decision (import + batch
  // edit share it). Draft only feeds blocks missing a preview decision;
  // cancelling discards the draft, preview decisions stay untouched.
  const [modalOrigen, setModalOrigen] = useState(null) // 'import' | 'edit' | null
  const [modalBloques, setModalBloques] = useState([])
  const [modalDecisiones, setModalDecisiones] = useState({})
  const [modalError, setModalError] = useState('')

  const hoy = isoLocal(new Date())
  const { celdas, desde, hasta } = useMemo(
    () => ventanaDeMes(mes.anio, mes.mes),
    [mes.anio, mes.mes],
  )

  // Un solo efecto por cambio de mes: agenda + actividad de la ventana
  // en paralelo. Las mutaciones reusan recargarVentana (mismo rango).
  async function recargarVentana() {
    try {
      const [ag, act] = await Promise.all([
        api.agendaRango(desde, hasta),
        api.actividad(desde, hasta),
      ])
      setItems((Array.isArray(ag) ? ag : []).map(normAgenda))
      const mapa = {}
      ;(Array.isArray(act) ? act : []).map(normActividad).forEach((x) => {
        if (x.fecha) mapa[x.fecha] = x.total
      })
      setActividad(mapa)
    } catch {
      setItems([])
      setActividad({})
    }
  }

  useEffect(() => { recargarVentana() }, [desde, hasta]) // eslint-disable-line react-hooks/exhaustive-deps

  // Vista año: agenda + actividad del año entero para marcar meses con contenido
  const [itemsAnio, setItemsAnio] = useState([])
  const [actividadAnio, setActividadAnio] = useState({})
  useEffect(() => {
    if (vista !== 'anio') return
    let vivo = true
    const d = `${mes.anio}-01-01`
    const h = `${mes.anio}-12-31`
    Promise.all([api.agendaRango(d, h), api.actividad(d, h)]).then(([ag, act]) => {
      if (!vivo) return
      setItemsAnio((Array.isArray(ag) ? ag : []).map(normAgenda))
      const mapa = {}
      ;(Array.isArray(act) ? act : []).map(normActividad).forEach((x) => {
        if (x.fecha) mapa[x.fecha] = x.total
      })
      setActividadAnio(mapa)
    }).catch(() => { if (vivo) { setItemsAnio([]); setActividadAnio({}) } })
    return () => { vivo = false }
  }, [vista, mes.anio])

  const porDia = useMemo(() => {
    const mapa = {}
    items.forEach((it) => {
      if (!it.fecha) return
      if (!mapa[it.fecha]) mapa[it.fecha] = []
      mapa[it.fecha].push(it)
    })
    return mapa
  }, [items])

  // Día expandido (modal): sin hora primero, igual que el ORDER BY del backend.
  const delDia = (diaAbierto ? (porDia[diaAbierto] || []) : []).slice().sort((a, b) =>
    (a.horaInicio || '').localeCompare(b.horaInicio || ''),
  )
  const sinHora = delDia.filter((it) => !it.horaInicio)
  const porHora = {}
  delDia.forEach((it) => {
    if (!it.horaInicio) return
    const h = Number(it.horaInicio.slice(0, 2))
    if (!porHora[h]) porHora[h] = []
    porHora[h].push(it)
  })
  const HORAS = Array.from({ length: 24 }, (_, h) => h)
  const hh = (h) => String(h).padStart(2, '0')
  const bloqueNota = (it) => (
    <div key={it.id} style={{ display: 'flex', alignItems: 'center', gap: 10, background: '#0A0A0A', border: '1px solid #1F1F1F', borderLeft: '3px solid #5FA97B', borderRadius: 6, padding: '8px 12px' }}>
      <span style={{ fontSize: 11, fontWeight: 700, color: '#C8C8C8', minWidth: 92 }}>{etiquetaHora(it)}</span>
      <div style={{ flex: 1, minWidth: 0 }}>
        <p style={{ margin: 0, fontSize: 12, color: '#EDEDED', overflow: 'hidden', textOverflow: 'ellipsis' }}>
          {it.texto || '(sin texto)'}
        </p>
        <p style={{ margin: '2px 0 0', fontSize: 10, color: it.carpetaNombre ? '#5FA97B' : '#8A8A8A' }}>
          {it.carpetaNombre ? `📁 ${it.carpetaNombre}` : 'Nota'}
        </p>
      </div>
      <button className="icon-btn" title="Eliminar nota" onClick={() => handleBorrar(it.id)}>
        <Icon name="trash" size={13} />
      </button>
    </div>
  )

  function moverMes(dir) {
    setMes((m) => {
      const d = new Date(m.anio, m.mes + dir, 1)
      return { anio: d.getFullYear(), mes: d.getMonth() }
    })
  }

  function irHoy() {
    const n = new Date()
    setMes({ anio: n.getFullYear(), mes: n.getMonth() })
    setSeleccionado(isoLocal(n))
    setVista('mes')
  }

  function elegirDia(iso) {
    setSeleccionado(iso)
    setDiaAbierto(iso)
    const d = parseISO(iso)
    // Días del mes vecino (en gris): el calendario viaja con vos.
    if (d.getFullYear() !== mes.anio || d.getMonth() !== mes.mes) {
      setMes({ anio: d.getFullYear(), mes: d.getMonth() })
    }
  }

  async function handleCrear(e) {
    e.preventDefault()
    const msg = validarNota({ texto, horaInicio, horaFin, carpetaId })
    if (msg) {
      setFormError(msg)
      return
    }
    try {
      setFormError('')
      await api.crearAgenda({
        fecha: seleccionado,
        texto: texto.trim(),
        hora_inicio: horaInicio,
        hora_fin: horaFin,
        carpeta_id: carpetaId ? Number(carpetaId) : null,
      })
      setTexto('')
      setHoraInicio('')
      setHoraFin('')
      recargarVentana()
    } catch (err) {
      setFormError(err.message || 'No se pudo guardar la nota')
    }
  }

  async function handleBorrar(id) {
    if (!window.confirm('¿Eliminar esta nota? Esta acción no se puede deshacer.')) return
    try {
      await api.borrarAgenda(id)
      recargarVentana()
    } catch (err) {
      setFormError(err.message || 'No se pudo borrar la nota')
    }
  }

  const normImport = (r) => ({
    total: r?.total ?? r?.Total ?? 0,
    ocurrencias: r?.ocurrencias ?? r?.Ocurrencias ?? [],
    avisos: r?.avisos ?? r?.Avisos ?? [],
    creadas: r?.creadas ?? r?.Creadas ?? 0,
    omitidas: r?.omitidas ?? r?.Omitidas ?? 0,
    bloques: (r?.bloques ?? r?.Bloques ?? []).map(normBloque),
    carpetasExistentes: (r?.carpetas_existentes ?? r?.carpetasExistentes ?? []).map((c) => ({
      id: c?.id ?? c?.ID ?? 0,
      nombre: c?.nombre ?? c?.Nombre ?? c?.name ?? '',
    })).filter((c) => c.nombre),
    carpetasCreadas: (r?.carpetas_creadas ?? r?.carpetasCreadas ?? []).map((c) => ({
      id: c?.id ?? c?.ID ?? 0,
      nombre: c?.nombre ?? c?.Nombre ?? '',
      bloques: c?.bloques ?? c?.Bloques ?? [],
    })),
  })

  // Bloque del menú por carpeta: claves en minúscula desde Go.
  const normBloque = (b) => ({
    indice: b?.indice ?? b?.Indice ?? 0,
    titulo: b?.titulo ?? b?.Titulo ?? '',
    carpetaPedida: b?.carpeta_pedida ?? b?.carpetaPedida ?? b?.CarpetaPedida ?? '',
    carpetaId: b?.carpeta_id ?? b?.carpetaId ?? b?.CarpetaID ?? null,
    estado: b?.estado ?? b?.Estado ?? '',
  })

  const etiquetaBloque = (b) => (b.titulo ? `Bloque ${b.indice} (${b.titulo})` : `Bloque ${b.indice}`)

  // Decisiones del menú por bloque: solo viajan los bloques tocados
  // (lo intacto es auto = comportamiento de siempre).
  // { [indice]: { modo: 'usar'|'crear'|'suelta', carpetaId?, nombre? } }
  // 'suelta' only comes from the overlay modal (explicit reject); the
  // side-by-side preview menu keeps auto = absent key.
  const decisionesPayload = (decisiones) =>
    Object.entries(decisiones)
      .map(([indice, d]) => {
        const bloque = Number(indice)
        if (d.modo === 'usar' && d.carpetaId) return { bloque, accion: 'usar', carpeta_id: Number(d.carpetaId) }
        if (d.modo === 'crear') return { bloque, accion: 'crear', nombre: (d.nombre ?? '').trim() }
        if (d.modo === 'suelta') return { bloque, accion: 'suelta' }
        return null
      })
      .filter(Boolean)
      .sort((a, b) => a.bloque - b.bloque)

  // All loose blocks (requested folder missing), in order.
  // Linked and sin-carpeta blocks never need the modal.
  // Separate from sueltosSinDecision: the overlay must open when ANY loose
  // block exists, even if the preview menu already preselected it.
  const bloquesSueltos = (preview) =>
    (preview?.bloques ?? [])
      .filter((b) => b.estado === 'suelta')
      .sort((a, b) => a.indice - b.indice)

  // Loose blocks still missing an explicit preview decision, in order.
  const sueltosSinDecision = (preview, decisiones) =>
    bloquesSueltos(preview).filter((b) => !decisiones[b.indice])

  // Opens the overlay when ANY loose block exists. Preview decisions are
  // preselected (usar/crear kept, missing defaults to explicit suelta) so
  // the inline menu acts only as a shortcut; the overlay is authoritative.
  // Returns true when the modal took over.
  function abrirModalSinCarpeta(origen, preview, decisiones) {
    const pendientes = bloquesSueltos(preview)
    if (pendientes.length === 0) return false
    const inicial = {}
    pendientes.forEach((b) => {
      const prev = decisiones?.[b.indice]
      if (prev?.modo === 'usar' && prev.carpetaId) inicial[b.indice] = { modo: 'usar', carpetaId: prev.carpetaId }
      else if (prev?.modo === 'crear') inicial[b.indice] = { modo: 'crear', nombre: (prev.nombre ?? '').trim() ? prev.nombre : (b.carpetaPedida ?? '') }
      else inicial[b.indice] = { modo: 'suelta' }
    })
    setModalBloques(pendientes)
    setModalDecisiones(inicial)
    setModalError('')
    setModalOrigen(origen)
    return true
  }

  function cerrarModalSinCarpeta() {
    setModalOrigen(null)
    setModalBloques([])
    setModalDecisiones({})
    setModalError('')
  }

  const normLote = (l) => ({
    id: l?.id ?? l?.ID ?? 0,
    nombre: l?.nombre ?? l?.Nombre ?? '',
    total: l?.total ?? l?.Total ?? 0,
    creado: l?.creado ?? l?.Creado ?? '',
    markdown: l?.markdown ?? l?.Markdown ?? '',
    carpetas: (l?.carpetas ?? l?.Carpetas ?? []).map((c) => ({
      id: c?.id ?? c?.ID ?? 0,
      nombre: c?.nombre ?? c?.Nombre ?? c?.name ?? '',
    })).filter((c) => c.nombre),
    ocurrencias: l?.ocurrencias ?? l?.Ocurrencias ?? [],
    avisos: l?.avisos ?? l?.Avisos ?? [],
  })

  // Folder fields travel lowercase from Go; accept capitalized just in case.
  const carpetaDeOcurrencia = (o) => ({
    nombre: o.carpeta_nombre ?? o.CarpetaNombre ?? o.carpetaNombre ?? '',
    id: o.carpeta_id ?? o.CarpetaID ?? o.carpetaId ?? null,
    estado: o.estado ?? o.Estado ?? '',
  })

  // Menú de carpeta por bloque: solo los bloques sueltos (carpeta pedida
  // que no existe) ofrecen elegir. Vinculados y sin carpeta solo informan.
  // decisiones vive afuera ({ [indice]: { modo, carpetaId?, nombre? } }).
  const menuCarpetasPorBloque = (bloques, carpetasDisponibles, decisiones, onDecision) => {
    if (!bloques || bloques.length === 0) return null
    const nombreVinculada = (b) =>
      (carpetasDisponibles.find((c) => String(c.id) === String(b.carpetaId))?.nombre) || b.carpetaPedida
    return (
      <div style={{ marginTop: 8 }}>
        <p className="panel-sub" style={{ fontWeight: 700, color: '#EDEDED' }}>Carpetas por bloque</p>
        <p className="panel-sub">Elegir acá preselecciona; la confirmación final es en el paso siguiente.</p>
        <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
          {bloques.map((b) => {
            const dec = decisiones[b.indice]
            const valorSelect = !dec || dec.modo === 'suelta' ? 'auto' : dec.modo === 'usar' && dec.carpetaId ? `usar:${dec.carpetaId}` : dec.modo === 'crear' ? 'crear' : 'auto'
            return (
              <div key={b.indice} style={{ background: '#0A0A0A', border: '1px solid #1F1F1F', borderRadius: 6, padding: '8px 12px' }}>
                <p style={{ margin: 0, fontSize: 12, color: '#EDEDED' }}>{etiquetaBloque(b)}</p>
                <p style={{ margin: '2px 0 0', fontSize: 11, color: b.estado === 'vinculada' ? '#5FA97B' : b.estado === 'suelta' ? '#C9964A' : '#8A8A8A' }}>
                  {b.estado === 'vinculada' && `✓ 📁 ${nombreVinculada(b)}`}
                  {b.estado === 'suelta' && `⚠ carpeta "${b.carpetaPedida}" no existe, queda suelto`}
                  {b.estado === 'sin-carpeta' && 'Sin carpeta'}
                </p>
                {b.estado === 'suelta' && (
                  <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap', marginTop: 6 }}>
                    <select
                      value={valorSelect}
                      onChange={(e) => {
                        const v = e.target.value
                        if (v === 'auto') onDecision(b.indice, null)
                        else if (v === 'crear') onDecision(b.indice, { modo: 'crear', nombre: dec?.nombre ?? b.carpetaPedida })
                        else onDecision(b.indice, { modo: 'usar', carpetaId: Number(v.slice(5)) })
                      }}
                      style={{ ...controlOscuro, flex: 1, minWidth: 160 }}
                    >
                      <option value="auto">Dejar suelto</option>
                      {carpetasDisponibles.map((c) => (
                        <option key={c.id} value={`usar:${c.id}`}>📁 {c.nombre}</option>
                      ))}
                      <option value="crear">Crear nueva…</option>
                    </select>
                    {dec?.modo === 'crear' && (
                      <input
                        value={dec.nombre ?? ''}
                        onChange={(e) => onDecision(b.indice, { modo: 'crear', nombre: e.target.value })}
                        placeholder="Nombre de la carpeta nueva…"
                        style={{ ...controlOscuro, flex: 1, minWidth: 160 }}
                      />
                    )}
                  </div>
                )}
              </div>
            )
          })}
        </div>
      </div>
    )
  }

  // Misma vista previa para importar y para editar: total + muestra + avisos.
  // Cada ocurrencia muestra ✓ vinculada (nombre carpeta) o ⚠ suelta con motivo.
  // menu es opcional: { bloques, carpetas, decisiones, onDecision }.
  const vistaPreviaImport = (datos, menu) => (
    datos && (
      <div style={{ marginTop: 8 }}>
        <p className="panel-sub">
          {datos.total} ocurrencia{datos.total !== 1 ? 's' : ''} (mostrando {datos.ocurrencias.length})
        </p>
        {datos.ocurrencias.map((o, i) => {
          const cf = carpetaDeOcurrencia(o)
          return (
            <p key={i} className="panel-sub" style={{ margin: '2px 0', color: '#C8C8C8' }}>
              {(o.fecha ?? o.Fecha) || ''} {(o.hora_inicio ?? o.HoraInicio) || ''}{(o.hora_fin ?? o.HoraFin) ? `–${o.hora_fin ?? o.HoraFin}` : ''} · {(o.texto ?? o.Texto) || ''}
              {cf.estado === 'vinculada' && cf.nombre && (
                <span style={{ color: '#5FA97B' }}>{` ✓ 📁 ${cf.nombre}`}</span>
              )}
              {cf.estado === 'suelta' && cf.nombre && (
                <span style={{ color: '#C9964A' }}>{` ⚠ carpeta "${cf.nombre}" no existe, queda suelto`}</span>
              )}
            </p>
          )
        })}
        {menu && menuCarpetasPorBloque(menu.bloques, menu.carpetas, menu.decisiones, menu.onDecision)}
        {datos.avisos.length > 0 && (
          <ul style={{ margin: '8px 0 0', paddingLeft: 18, fontSize: 11, color: '#C9964A' }}>
            {datos.avisos.map((a, i) => <li key={i}>{a}</li>)}
          </ul>
        )}
      </div>
    )
  )

  async function recargarLotes() {
    try {
      setLotesError('')
      const ls = await api.listarLotes()
      setLotes((Array.isArray(ls) ? ls : []).map(normLote))
    } catch (err) {
      setLotesError(err.message || 'No se pudieron leer los lotes')
    }
  }

  useEffect(() => { recargarLotes() }, []) // eslint-disable-line react-hooks/exhaustive-deps

  // Copy button for the AI prompt: clipboard with textarea fallback.
  async function handleCopiarPrompt() {
    const ok = await copiarTexto(PROMPT_IMPORT_MD)
    setPromptCopiado(ok)
    if (ok) setTimeout(() => setPromptCopiado(false), 2000)
  }

  async function handleImportPreview() {
    if (!importTexto.trim() || importando) return
    try {
      setImportando(true)
      setImportError('')
      setImportResultado(null)
      setImportDecisiones({})
      const res = await api.importarAgendaMD(importTexto, true)
      setImportPreview(normImport(res))
    } catch (err) {
      setImportError(err.message || 'No se pudo previsualizar')
    } finally {
      setImportando(false)
    }
  }

  // Carpetas para el menú: las del preview (frescas) o las globales.
  const carpetasParaMenu = (preview) =>
    (preview?.carpetasExistentes?.length > 0 ? preview.carpetasExistentes : carpetas)

  const cambiarDecision = (setDecisiones) => (indice, dec) => {
    setDecisiones((prev) => {
      const copia = { ...prev }
      if (!dec) delete copia[indice]
      else copia[indice] = dec
      return copia
    })
  }

  function validarDecisiones(decisiones) {
    for (const [indice, d] of Object.entries(decisiones)) {
      if (d.modo === 'crear' && !(d.nombre ?? '').trim()) {
        return `Escribí un nombre para la carpeta nueva del bloque ${indice}`
      }
      if (d.modo === 'usar' && !d.carpetaId) {
        return `Elegí una carpeta existente para el bloque ${indice}`
      }
    }
    return ''
  }

  // Direct persist (no pending loose blocks, or modal already merged).
  async function ejecutarImportConfirm(decisiones) {
    try {
      setImportando(true)
      setImportError('')
      const res = await api.importarAgendaMD(importTexto, false, importNombre, decisionesPayload(decisiones))
      const norm = normImport(res)
      setImportResultado(norm)
      setImportPreview(null)
      setImportDecisiones({})
      setImportTexto('')
      setImportNombre('')
      recargarLotes()
      recargarVentana()
    } catch (err) {
      setImportError(err.message || 'No se pudo importar')
    } finally {
      setImportando(false)
    }
  }

  async function handleImportConfirm() {
    if (!importTexto.trim() || importando) return
    const msgDec = validarDecisiones(importDecisiones)
    if (msgDec) {
      setImportError(msgDec)
      return
    }
    // Authoritative overlay: opens whenever ANY loose block exists, even
    // with preview decisions (they arrive preselected). Direct confirm
    // only when no block is loose.
    let preview = importPreview
    if (!preview) {
      try {
        setImportando(true)
        setImportError('')
        const res = await api.importarAgendaMD(importTexto, true)
        preview = normImport(res)
        setImportPreview(preview)
      } catch (err) {
        setImportError(err.message || 'No se pudo previsualizar')
        setImportando(false)
        return
      }
      setImportando(false)
    }
    if (preview && abrirModalSinCarpeta('import', preview, importDecisiones)) return
    await ejecutarImportConfirm(importDecisiones)
  }

  async function handleEditarLote(lote) {
    if (loteBusy) return
    try {
      setLoteBusy(true)
      setEditError('')
      setEditPreview(null)
      const d = await api.obtenerLote(lote.id)
      const n = normLote(d)
      setEditId(n.id)
      setEditNombre(n.nombre)
      setEditTexto(n.markdown)
    } catch (err) {
      setLotesError(err.message || 'No se pudo abrir el lote')
    } finally {
      setLoteBusy(false)
    }
  }

  function handleCancelarEdicion() {
    setEditId(null)
    setEditNombre('')
    setEditTexto('')
    setEditPreview(null)
    setEditError('')
    setEditDecisiones({})
  }

  async function handleEditPreview() {
    if (!editTexto.trim() || loteBusy) return
    try {
      setLoteBusy(true)
      setEditError('')
      setEditDecisiones({})
      const res = await api.importarAgendaMD(editTexto, true)
      setEditPreview(normImport(res))
    } catch (err) {
      setEditError(err.message || 'No se pudo previsualizar')
    } finally {
      setLoteBusy(false)
    }
  }

  // Direct persist for batch edit (no pending loose blocks, or merged).
  async function ejecutarGuardarLote(decisiones) {
    try {
      setLoteBusy(true)
      setEditError('')
      const res = await api.actualizarLote(editId, { nombre: editNombre, markdown: editTexto, decisiones: decisionesPayload(decisiones) })
      handleCancelarEdicion()
      setImportResultado(normImport(res))
      recargarLotes()
      recargarVentana()
    } catch (err) {
      setEditError(err.message || 'No se pudo guardar el lote')
    } finally {
      setLoteBusy(false)
    }
  }

  async function handleGuardarLote() {
    if (!editTexto.trim() || loteBusy || editId == null) return
    const msgDec = validarDecisiones(editDecisiones)
    if (msgDec) {
      setEditError(msgDec)
      return
    }
    // Same authoritative overlay rule as import confirm.
    let preview = editPreview
    if (!preview) {
      try {
        setLoteBusy(true)
        setEditError('')
        const res = await api.importarAgendaMD(editTexto, true)
        preview = normImport(res)
        setEditPreview(preview)
      } catch (err) {
        setEditError(err.message || 'No se pudo previsualizar')
        setLoteBusy(false)
        return
      }
      setLoteBusy(false)
    }
    if (preview && abrirModalSinCarpeta('edit', preview, editDecisiones)) return
    await ejecutarGuardarLote(editDecisiones)
  }

  // Overlay confirm: validates non-empty names for crear and chosen folder
  // for usar, then persists with preview + modal decisions combined (modal
  // carries every loose block preselected, so it is authoritative).
  async function handleModalConfirm() {
    for (const [indice, d] of Object.entries(modalDecisiones)) {
      if (d.modo === 'crear' && !(d.nombre ?? '').trim()) {
        setModalError(`Escribí un nombre para la carpeta nueva del bloque ${indice}`)
        return
      }
      if (d.modo === 'usar' && !d.carpetaId) {
        setModalError(`Elegí una carpeta existente para el bloque ${indice}`)
        return
      }
    }
    if (modalOrigen === 'import') {
      const combinadas = { ...importDecisiones, ...modalDecisiones }
      const msgDec = validarDecisiones(combinadas)
      if (msgDec) {
        setModalError(msgDec)
        return
      }
      cerrarModalSinCarpeta()
      await ejecutarImportConfirm(combinadas)
    } else if (modalOrigen === 'edit') {
      const combinadas = { ...editDecisiones, ...modalDecisiones }
      const msgDec = validarDecisiones(combinadas)
      if (msgDec) {
        setModalError(msgDec)
        return
      }
      cerrarModalSinCarpeta()
      await ejecutarGuardarLote(combinadas)
    }
  }

  const cambiarModalDecision = (indice, dec) => {
    setModalDecisiones((prev) => ({ ...prev, [indice]: dec }))
  }

  async function handleBorrarLote(lote) {
    if (loteBusy) return
    if (!window.confirm(`¿Eliminar "${lote.nombre}" y sus ${lote.total} evento(s)? Tus notas a mano no se tocan.`)) return
    try {
      setLoteBusy(true)
      await api.borrarLote(lote.id)
      if (editId === lote.id) handleCancelarEdicion()
      recargarLotes()
      recargarVentana()
    } catch (err) {
      setLotesError(err.message || 'No se pudo borrar el lote')
    } finally {
      setLoteBusy(false)
    }
  }

  return (
    <div>
      {vista === 'anio' ? (
      <div className="panel">
        <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 12 }}>
          <button className="icon-btn" title="Año anterior" onClick={() => setMes((m) => ({ ...m, anio: m.anio - 1 }))}>‹</button>
          <p className="panel-title" style={{ margin: 0, flex: 1, textAlign: 'center' }}>
            {mes.anio}
          </p>
          <button className="icon-btn" title="Año siguiente" onClick={() => setMes((m) => ({ ...m, anio: m.anio + 1 }))}>›</button>
          <button className="btn-outline" onClick={irHoy}>Hoy</button>
        </div>
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(3, 1fr)', gap: 8 }}>
          {MESES.map((nombre, i) => {
            const pref = `${mes.anio}-${String(i + 1).padStart(2, '0')}`
            const notasM = itemsAnio.filter((it) => (it.fecha || '').startsWith(pref)).length
            const histM = Object.entries(actividadAnio).some(([f, t]) => f.startsWith(pref) && t > 0)
            const esActual = i === mes.mes
            return (
              <button
                key={i}
                onClick={() => { setMes((m) => ({ ...m, mes: i })); setVista('mes') }}
                title={`${nombre} de ${mes.anio}${notasM ? ` · ${notasM} nota${notasM > 1 ? 's' : ''}` : ''}`}
                style={{
                  display: 'flex', flexDirection: 'column', alignItems: 'flex-start', gap: 4,
                  padding: '10px 12px', borderRadius: 6, cursor: 'pointer', textAlign: 'left',
                  background: esActual ? '#1A1A1A' : 'transparent',
                  border: esActual ? '1px solid #C8C8C8' : '1px solid #2A2A2A',
                  color: '#EDEDED', fontFamily: 'inherit', fontSize: 12,
                }}
              >
                <span style={{ textTransform: 'capitalize', fontWeight: esActual ? 700 : 400 }}>{nombre}</span>
                <span style={{ display: 'flex', gap: 6, fontSize: 10, color: '#8A8A8A', minHeight: 14 }}>
                  {histM && <span title="Historia">● historia</span>}
                  {notasM > 0 && <span title="Notas" style={{ color: '#5FA97B' }}>● {notasM} nota{notasM > 1 ? 's' : ''}</span>}
                  {!histM && notasM === 0 && <span> </span>}
                </span>
              </button>
            )
          })}
        </div>
      </div>
      ) : (
      <div className="panel">
        <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 12 }}>
          <button className="icon-btn" title="Mes anterior" onClick={() => moverMes(-1)}>‹</button>
          <button
            className="panel-title"
            title="Ver el año"
            onClick={() => setVista('anio')}
            style={{ margin: 0, flex: 1, textAlign: 'center', textTransform: 'capitalize', background: 'transparent', border: 'none', cursor: 'pointer', fontFamily: 'inherit' }}
          >
            {MESES[mes.mes]} {mes.anio}
          </button>
          <button className="icon-btn" title="Mes siguiente" onClick={() => moverMes(1)}>›</button>
          <button className="btn-outline" onClick={irHoy}>Hoy</button>
          <button className="btn-outline" onClick={() => setImportAbierto((v) => !v)}>
            {importAbierto ? 'Cerrar import' : 'Importar MD'}
          </button>
        </div>
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(7, 1fr)', gap: 4 }}>
          {DIAS_CORTO.map((d, i) => (
            <div key={i} style={{ textAlign: 'center', fontSize: 10, fontWeight: 700, color: '#5A5A5A', padding: '4px 0' }}>
              {d}
            </div>
          ))}
          {celdas.map((d) => {
            const iso = isoLocal(d)
            const fuera = d.getMonth() !== mes.mes
            const esHoy = iso === hoy
            const activo = iso === seleccionado
            const notas = porDia[iso]?.length ?? 0
            const historia = (actividad[iso] ?? 0) > 0
            const vistaPrevia = (porDia[iso] || []).slice().sort((a, b) =>
              (a.horaInicio || '').localeCompare(b.horaInicio || ''),
            ).slice(0, 2)
            return (
              <button
                key={iso}
                onClick={() => elegirDia(iso)}
                title={`${tituloDia(iso)}${notas ? ` · ${notas} nota${notas > 1 ? 's' : ''}` : ''}`}
                style={{
                  display: 'flex', flexDirection: 'column', alignItems: 'stretch', gap: 3,
                  padding: '5px 6px', borderRadius: 6, cursor: 'pointer', minHeight: 62, textAlign: 'left',
                  background: activo ? '#1A1A1A' : esHoy ? '#141414' : 'transparent',
                  border: activo ? '1px solid #C8C8C8' : esHoy ? '1px solid #2A2A2A' : '1px solid transparent',
                  color: fuera ? '#5A5A5A' : '#EDEDED',
                  fontFamily: 'inherit', fontSize: 12,
                }}
              >
                <span style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                  <span style={{ fontWeight: esHoy || activo ? 700 : 400 }}>{d.getDate()}</span>
                  <span style={{ display: 'flex', gap: 3 }}>
                    {historia && <span style={{ width: 5, height: 5, borderRadius: '50%', background: '#8A8A8A' }} />}
                    {notas > 0 && <span style={{ width: 5, height: 5, borderRadius: '50%', background: '#5FA97B' }} />}
                  </span>
                </span>
                {vistaPrevia.map((it) => (
                  <span
                    key={it.id}
                    title={`${etiquetaHora(it)} · ${it.texto || it.carpetaNombre || 'Nota'}`}
                    style={{ fontSize: 9, lineHeight: 1.5, padding: '0 4px', borderRadius: 3, background: 'rgba(95,169,123,0.15)', color: '#C8C8C8', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}
                  >
                    {it.horaInicio ? `${it.horaInicio} ` : ''}{it.texto || it.carpetaNombre || 'Nota'}
                  </span>
                ))}
                {notas > 2 && (
                  <span style={{ fontSize: 9, color: '#8A8A8A', padding: '0 4px' }}>+{notas - 2} más</span>
                )}
              </button>
            )
          })}
        </div>
      </div>
      )}

      {importAbierto && (
      <div className="panel" style={{ marginTop: 12 }}>
        <p className="panel-title">Importar compromisos desde Markdown</p>
        <p className="panel-sub">Pega bloques ## Compromiso: con Tipo, Fecha/Desde-Hasta y Horario. Agrega - Carpeta: Nombre para alinear con una carpeta: si no existe, la creas o la elegís por bloque en la vista previa. Primero previsualiza sin guardar, después confirma.</p>
        {/* Two columns side by side (stack on narrow): form left, live preview right. */}
        <div style={{ display: 'flex', gap: 24, flexWrap: 'wrap', alignItems: 'flex-start' }}>
          <div style={{ flex: '1 1 260px', minWidth: 0 }}>
            <input
              value={importNombre}
              onChange={(e) => setImportNombre(e.target.value)}
              placeholder="Nombre (opcional, Ej: Cursada 2026)"
              disabled={importando}
              style={{ ...controlOscuro, width: '100%', marginBottom: 8, boxSizing: 'border-box' }}
            />
            <textarea
              className="import-area"
              value={importTexto}
              onChange={(e) => setImportTexto(e.target.value)}
              placeholder={'## Compromiso: Física I\n- Tipo: semanal\n- Desde: 2026-03-02\n- Hasta: 2026-03-15\n- Horario: lunes 19-20, miércoles 19-20\n- Aula: 101\n- Docente: García\n- Carpeta: Programación II'}
              rows={8}
              disabled={importando}
            />
            <div className="export-grid">
              <button
                className="btn-outline"
                disabled={!importTexto.trim() || importando}
                onClick={handleImportPreview}
              >
                {importando ? 'Procesando…' : 'Vista previa'}
              </button>
              <button
                className="btn-solid"
                disabled={!importTexto.trim() || importando}
                onClick={handleImportConfirm}
              >
                Confirmar
              </button>
            </div>
            {importError && (
              <p style={{ fontSize: 11, color: '#ff9999', margin: '8px 0 0' }}>{importError}</p>
            )}
          </div>
          <div style={{ flex: '1 1 320px', minWidth: 0 }}>
            <p className="panel-sub" style={{ fontWeight: 700, color: '#EDEDED' }}>Prompt para tu IA</p>
            <div className="export-out" style={{ marginTop: 0 }}>
              <button className="btn-outline" onClick={handleCopiarPrompt}>
                {promptCopiado ? 'Copiado' : 'Copiar'}
              </button>
              <pre className="export-pre">{PROMPT_IMPORT_MD}</pre>
            </div>
            <p className="panel-sub" style={{ fontWeight: 700, color: '#EDEDED', marginTop: 12 }}>Vista previa</p>
            {!importPreview && !importResultado && (
              <p className="panel-sub">Pegá tu MD y dale Vista previa: acá verás el resultado antes de guardar.</p>
            )}
            {vistaPreviaImport(importPreview, importPreview && {
              bloques: importPreview.bloques,
              carpetas: carpetasParaMenu(importPreview),
              decisiones: importDecisiones,
              onDecision: cambiarDecision(setImportDecisiones),
            })}
            {importResultado && (
              <div style={{ marginTop: 8 }}>
                <p className="panel-sub">
                  {importResultado.creadas} creadas · {importResultado.omitidas} omitidas
                </p>
                {(importResultado.carpetasCreadas?.length > 0) && (
                  <p className="panel-sub" style={{ color: '#5FA97B' }}>
                    {importResultado.carpetasCreadas.map((c) => `📁 Carpeta nueva: ${c.nombre} (bloque${c.bloques.length !== 1 ? 's' : ''} ${c.bloques.join(', ')})`).join(' · ')}
                  </p>
                )}
                {importResultado.avisos.length > 0 && (
                  <ul style={{ margin: '8px 0 0', paddingLeft: 18, fontSize: 11, color: '#C9964A' }}>
                    {importResultado.avisos.map((a, i) => <li key={i}>{a}</li>)}
                  </ul>
                )}
              </div>
            )}
          </div>
        </div>
      </div>
      )}

      <div className="panel" style={{ marginTop: 12 }}>
        <p className="panel-title">MD cargados</p>
        <p className="panel-sub">Cada import guarda su fuente: editala o eliminala sin tocar tus notas a mano.</p>
        {lotesError && (
          <p style={{ fontSize: 11, color: '#ff9999', margin: '8px 0 0' }}>{lotesError}</p>
        )}
        {lotes.length === 0 ? (
          <p className="panel-sub">Todavía no hay MD cargados: importá el primero arriba.</p>
        ) : (
          <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
            {lotes.map((l) => (
              <div key={l.id} style={{ display: 'flex', alignItems: 'center', gap: 10, background: '#0A0A0A', border: '1px solid #1F1F1F', borderRadius: 6, padding: '8px 12px' }}>
                <div style={{ flex: 1, minWidth: 0 }}>
                  <p style={{ margin: 0, fontSize: 12, color: '#EDEDED', overflow: 'hidden', textOverflow: 'ellipsis' }}>
                    {l.nombre}
                  </p>
                  <p style={{ margin: '2px 0 0', fontSize: 10, color: '#8A8A8A' }}>
                    {l.total} evento{l.total !== 1 ? 's' : ''} · cargado {l.creado || '—'}
                  </p>
                  {(l.carpetas?.length > 0) && (
                    <p style={{ margin: '2px 0 0', fontSize: 10, color: '#5FA97B' }}>
                      {l.carpetas.map((c) => `📁 ${c.nombre}`).join(' · ')}
                    </p>
                  )}
                </div>
                <button className="btn-outline" disabled={loteBusy} onClick={() => handleEditarLote(l)}>
                  Editar
                </button>
                <button className="btn-outline" disabled={loteBusy} onClick={() => handleBorrarLote(l)}>
                  Eliminar
                </button>
              </div>
            ))}
          </div>
        )}
        {editId !== null && (
          <div style={{ marginTop: 8, borderTop: '1px solid #1F1F1F', paddingTop: 8 }}>
            <p className="panel-sub">Editando lote #{editId}: cambiá el markdown, previsualizá y guardá.</p>
            <input
              value={editNombre}
              onChange={(e) => setEditNombre(e.target.value)}
              placeholder="Nombre del lote"
              disabled={loteBusy}
              style={{ ...controlOscuro, width: '100%', marginBottom: 8, boxSizing: 'border-box' }}
            />
            <textarea
              className="import-area"
              value={editTexto}
              onChange={(e) => setEditTexto(e.target.value)}
              rows={8}
              disabled={loteBusy}
            />
            <div className="export-grid">
              <button
                className="btn-outline"
                disabled={!editTexto.trim() || loteBusy}
                onClick={handleEditPreview}
              >
                {loteBusy ? 'Procesando…' : 'Vista previa'}
              </button>
              <button
                className="btn-solid"
                disabled={!editTexto.trim() || loteBusy}
                onClick={handleGuardarLote}
              >
                Guardar
              </button>
              <button
                className="btn-outline"
                disabled={loteBusy}
                onClick={handleCancelarEdicion}
              >
                Cancelar
              </button>
            </div>
            {editError && (
              <p style={{ fontSize: 11, color: '#ff9999', margin: '8px 0 0' }}>{editError}</p>
            )}
            {vistaPreviaImport(editPreview, editPreview && {
              bloques: editPreview.bloques,
              carpetas: carpetasParaMenu(editPreview),
              decisiones: editDecisiones,
              onDecision: cambiarDecision(setEditDecisiones),
            })}
          </div>
        )}
      </div>

      {modalOrigen && (
      <div className="modal-overlay animate-fade-in">
        <div className="modal animate-slide-up" style={{ maxWidth: 560, maxHeight: '85vh', overflowY: 'auto' }} onClick={(e) => e.stopPropagation()}>
          <div className="modal-head">
            <h2 className="modal-title">Bloques sin carpeta</h2>
            <button type="button" className="modal-close" title="Cancelar" disabled={importando || loteBusy} onClick={cerrarModalSinCarpeta}>
              <Icon name="x" size={14} />
            </button>
          </div>
          <p className="panel-sub">Estos bloques piden una carpeta que no existe. Elegí qué hacer con cada uno antes de confirmar.</p>
          <div style={{ display: 'flex', flexDirection: 'column', gap: 8, marginTop: 8 }}>
            {modalBloques.map((b) => {
              const dec = modalDecisiones[b.indice] ?? { modo: 'suelta' }
              const carpetasModal = carpetasParaMenu(modalOrigen === 'import' ? importPreview : editPreview)
              return (
                <div key={b.indice} style={{ background: '#0A0A0A', border: '1px solid #1F1F1F', borderRadius: 6, padding: '8px 12px' }}>
                  <p style={{ margin: 0, fontSize: 12, color: '#EDEDED' }}>{b.carpetaPedida ? `${etiquetaBloque(b)}: falta carpeta "${b.carpetaPedida}" (no encontrada)` : `${etiquetaBloque(b)}: sin nombre de carpeta`}</p>
                  <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap', marginTop: 6 }}>
                    <select
                      value={dec.modo}
                      onChange={(e) => {
                        const v = e.target.value
                        if (v === 'crear') cambiarModalDecision(b.indice, { modo: 'crear', nombre: dec.nombre ?? b.carpetaPedida ?? '', carpetaId: dec.carpetaId })
                        else if (v === 'usar') cambiarModalDecision(b.indice, { modo: 'usar', carpetaId: dec.carpetaId ?? '', nombre: dec.nombre ?? b.carpetaPedida ?? '' })
                        else cambiarModalDecision(b.indice, { modo: 'suelta' })
                      }}
                      disabled={importando || loteBusy}
                      style={{ ...controlOscuro, flex: 1, minWidth: 160 }}
                    >
                      <option value="usar">Elegir existente…</option>
                      <option value="crear">Crear nueva…</option>
                      <option value="suelta">Dejar suelto</option>
                    </select>
                    {dec.modo === 'usar' && (
                      <select
                        value={dec.carpetaId ?? ''}
                        onChange={(e) => cambiarModalDecision(b.indice, { modo: 'usar', carpetaId: e.target.value ? Number(e.target.value) : '', nombre: dec.nombre })}
                        disabled={importando || loteBusy}
                        style={{ ...controlOscuro, flex: 1, minWidth: 160 }}
                      >
                        <option value="">Elegir carpeta…</option>
                        {carpetasModal.map((c) => (
                          <option key={c.id} value={c.id}>📁 {c.nombre}</option>
                        ))}
                      </select>
                    )}
                    {dec.modo === 'crear' && (
                      <input
                        value={dec.nombre ?? ''}
                        onChange={(e) => cambiarModalDecision(b.indice, { modo: 'crear', nombre: e.target.value, carpetaId: dec.carpetaId })}
                        placeholder="Nombre de la carpeta nueva…"
                        disabled={importando || loteBusy}
                        style={{ ...controlOscuro, flex: 1, minWidth: 160 }}
                      />
                    )}
                  </div>
                </div>
              )
            })}
          </div>
          {modalError && (
            <p style={{ fontSize: 11, color: '#ff9999', margin: '8px 0 0' }}>{modalError}</p>
          )}
          <div className="modal-actions">
            <button type="button" className="btn-ghost" disabled={importando || loteBusy} onClick={cerrarModalSinCarpeta}>Cancelar</button>
            <button type="button" className="btn-solid" disabled={importando || loteBusy} onClick={handleModalConfirm}>Confirmar</button>
          </div>
        </div>
      </div>
      )}

      {diaAbierto && (
      <div className="modal-overlay animate-fade-in" onClick={() => setDiaAbierto(null)}>
        <div className="modal animate-slide-up" style={{ maxWidth: 640, maxHeight: '85vh', overflowY: 'auto' }} onClick={(e) => e.stopPropagation()}>
          <div className="modal-head">
            <h2 className="modal-title" style={{ textTransform: 'capitalize' }}>{tituloDia(diaAbierto)}</h2>
            <button type="button" className="modal-close" onClick={() => setDiaAbierto(null)}>
              <Icon name="x" size={14} />
            </button>
          </div>
          <p className="panel-sub">
            {delDia.length === 0
              ? 'Día libre: agendá la primera cosa con el formulario.'
              : `${delDia.length} cosa${delDia.length > 1 ? 's' : ''} este día, hora por hora.`}
          </p>
        <form onSubmit={handleCrear}>
          <div className="inline-form" style={{ marginBottom: 8 }}>
            <input
              value={texto}
              onChange={(e) => setTexto(e.target.value)}
              placeholder="Nueva nota para este día…"
            />
          </div>
          <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap', marginBottom: 8 }}>
            <input
              type="time"
              value={horaInicio}
              onChange={(e) => setHoraInicio(e.target.value)}
              title="Hora de inicio (opcional)"
              style={controlOscuro}
            />
            <input
              type="time"
              value={horaFin}
              onChange={(e) => setHoraFin(e.target.value)}
              title="Hora de fin (opcional)"
              style={controlOscuro}
            />
            <select
              value={carpetaId}
              onChange={(e) => setCarpetaId(e.target.value)}
              title="Carpeta (opcional)"
              style={{ ...controlOscuro, flex: 1, minWidth: 140 }}
            >
              <option value="">Sin carpeta</option>
              {carpetas.map((c) => (
                <option key={c.id} value={c.id}>{c.nombre}</option>
              ))}
            </select>
            <button type="submit" className="btn-solid">Añadir</button>
          </div>
          {formError && (
            <p style={{ fontSize: 11, color: '#ff9999', margin: '0 0 4px' }}>{formError}</p>
          )}
        </form>
        {sinHora.length > 0 && (
          <div style={{ margin: '12px 0 4px' }}>
            <p className="panel-sub" style={{ fontWeight: 700, color: '#EDEDED' }}>Sin hora</p>
            <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
              {sinHora.map(bloqueNota)}
            </div>
          </div>
        )}
        <div style={{ marginTop: 8 }}>
          {HORAS.map((h) => (
            <div key={h} style={{ display: 'flex', gap: 10, borderTop: '1px solid #1F1F1F', padding: '6px 0', minHeight: 34 }}>
              <span style={{ width: 40, flexShrink: 0, fontSize: 10, color: '#5A5A5A', paddingTop: 8 }}>{hh(h)}:00</span>
              <div style={{ flex: 1, minWidth: 0, display: 'flex', flexDirection: 'column', gap: 6 }}>
                {(porHora[h] || []).map(bloqueNota)}
              </div>
            </div>
          ))}
        </div>
        </div>
      </div>
      )}
    </div>
  )
}

/* ───────── APP ───────── */
export default function App() {
  const [view, setView] = useState('all')
  const [query, setQuery] = useState('')
  const [modalOpen, setModalOpen] = useState(false)

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

  // (La carpeta abierta vive dentro de Carpetas; ver FoldersView.abiertaId.)

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

  return (
    <div className="app">
      <Sidebar currentView={view} onChangeView={changeView} counts={counts} />
      <main className="main">
        <Topbar query={query} onQueryChange={setQuery} onAdd={() => setModalOpen(true)} showSearch={view !== 'all'} />
        <div className="content">
          <PageHeader title={pageTitle} subtitle={pageSubtitle} showFilter={false} />

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
          ) : view === 'all' ? (
            <>
              <ProgressOverview stats={stats} />
              <AgendaView carpetas={carpetas} />
            </>
          ) : view === 'dashboard' ? (
            <DashboardView
              stats={stats}
              carpetas={carpetas}
              progresos={progresos}
              recursos={recursos}
              onGoExport={() => setView('export')}
            />
          ) : view === 'folders' ? (
            <FoldersView
              carpetas={carpetas}
              progresos={progresos}
              recursos={recursos}
              folderById={folderById}
              onCycleStatus={handleCycleStatus}
              onDelete={handleDelete}
              onOpen={handleOpen}
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
