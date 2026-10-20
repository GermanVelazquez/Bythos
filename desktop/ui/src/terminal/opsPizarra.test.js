import { describe, it, expect, vi } from 'vitest'

vi.mock('./excalidrawAdapter.js', () => ({ convertirElementos: (lista) => lista }))

const { aplicarOps, calcularInversa, camposDeshacer, planDeshacer, versionesDe } = await import('./opsPizarra.js')

const el = (id, extra = {}) => ({ id, type: 'rectangle', x: 0, y: 0, version: 1, versionNonce: 1, isDeleted: false, ...extra })
const agregar = (id, extra = {}) => ({ accion: 'agregar', elemento: { id, tipo: 'rectangle', x: 5, y: 5, ...extra } })

describe('calcularInversa', () => {
  it('agregado -> quitar', () => {
    const antes = [el('a')]
    const { elementos } = aplicarOps(antes, [agregar('b')])
    expect(calcularInversa(antes, elementos)).toEqual({ restaurar: [], quitar: ['b'] })
  })

  it('cambiado -> restaura el estado previo', () => {
    const a = el('a')
    const { elementos } = aplicarOps([a], [{ accion: 'mover', ids: ['a'], dx: 10, dy: 0 }])
    const inv = calcularInversa([a], elementos)
    expect(inv.quitar).toEqual([])
    expect(inv.restaurar).toEqual([a])
  })

  it('borrado -> restaura el elemento', () => {
    const a = el('a')
    const { elementos } = aplicarOps([a], [{ accion: 'borrar', ids: ['a'] }])
    expect(calcularInversa([a], elementos).restaurar).toEqual([a])
  })

  it('una lápida tocada se restaura, no se quita', () => {
    const lapida = el('a', { isDeleted: true })
    const { elementos } = aplicarOps([lapida], [agregar('a')])
    expect(calcularInversa([lapida], elementos)).toEqual({ restaurar: [lapida], quitar: [] })
  })

  it('identidad y lote sin cambios dan inversa vacía', () => {
    const antes = [el('a')]
    expect(calcularInversa(antes, antes)).toEqual({ restaurar: [], quitar: [] })
    const { elementos } = aplicarOps(antes, [])
    expect(calcularInversa(antes, elementos)).toEqual({ restaurar: [], quitar: [] })
  })

  it('un lote de marcas se quita entero', () => {
    const { elementos } = aplicarOps([], [
      agregar('marca-abcdefghij-m', { datos: { bythos: 'marca' } }),
      agregar('marca-abcdefghij-t'),
    ])
    expect(calcularInversa([], elementos).quitar).toEqual(['marca-abcdefghij-m', 'marca-abcdefghij-t'])
  })
})

describe('planDeshacer', () => {
  const escenario = () => {
    const caja = el('c')
    const texto = el('t', { type: 'text', containerId: 'c' })
    const antes = [caja, texto]
    const { elementos } = aplicarOps(antes, [{ accion: 'mover', ids: ['c'], dx: 1, dy: 1 }])
    const inversa = calcularInversa(antes, elementos)
    return { elementos, inversa, versiones: versionesDe(elementos, inversa) }
  }

  it('restaura con version actual + 1', () => {
    const { elementos, inversa, versiones } = escenario()
    const r = planDeshacer(elementos, inversa, versiones)
    const c = r.elementos.find((e) => e.id === 'c')
    expect(r.omitidos).toBe(0)
    expect(r.aviso).toBeNull()
    expect(c.x).toBe(0)
    expect(c.version).toBe(elementos.find((e) => e.id === 'c').version + 1)
  })

  it('quitar marca el elemento como borrado', () => {
    const { elementos } = aplicarOps([], [agregar('n')])
    const inversa = calcularInversa([], elementos)
    const r = planDeshacer(elementos, inversa, versionesDe(elementos, inversa))
    expect(r.elementos.find((e) => e.id === 'n').isDeleted).toBe(true)
  })

  it('un contenedor y sus textos se omiten juntos y cuentan elementos', () => {
    const { elementos, inversa, versiones } = escenario()
    const tocado = elementos.map((e) => (e.id === 't' ? { ...e, versionNonce: 999 } : e))
    const r = planDeshacer(tocado, inversa, versiones)
    expect(r.omitidos).toBe(2)
    expect(r.aviso).toBe('2 elementos no se restauraron porque los cambiaste')
    expect(r.elementos.find((e) => e.id === 'c').x).toBe(1)
  })

  it('un cambio solo de version también es conflicto', () => {
    const { elementos, inversa, versiones } = escenario()
    const tocado = elementos.map((e) => (e.id === 'c' ? { ...e, version: e.version + 1 } : e))
    const r = planDeshacer(tocado, inversa, versiones)
    expect(r.omitidos).toBe(2)
    expect(r.elementos.find((e) => e.id === 'c').x).toBe(1)
  })

  it('deshacer parcial: restaura lo intacto y respeta lo que cambió el usuario', () => {
    const antes = [el('a'), el('b')]
    const { elementos } = aplicarOps(antes, [{ accion: 'mover', ids: ['a', 'b'], dx: 10, dy: 0 }])
    const inversa = calcularInversa(antes, elementos)
    const versiones = versionesDe(elementos, inversa)
    const movidoPorUsuario = { ...elementos.find((e) => e.id === 'a'), x: 50, version: 99 }
    const actuales = elementos.map((e) => (e.id === 'a' ? movidoPorUsuario : e))
    const r = planDeshacer(actuales, inversa, versiones)
    expect(r.omitidos).toBe(1)
    expect(r.aviso).toBe('1 elemento no se restauró porque lo cambiaste')
    expect(r.elementos.find((e) => e.id === 'a')).toBe(movidoPorUsuario)
    expect(r.elementos.find((e) => e.id === 'b').x).toBe(0)
  })

  it('el mensaje plural cuenta todos los elementos de la unidad', () => {
    const antes = [el('c'), el('t1', { type: 'text', containerId: 'c' }), el('t2', { type: 'text', containerId: 'c' })]
    const { elementos } = aplicarOps(antes, [{ accion: 'mover', ids: ['c'], dx: 1, dy: 1 }])
    const inversa = calcularInversa(antes, elementos)
    const tocado = elementos.map((e) => (e.id === 't2' ? { ...e, versionNonce: 999 } : e))
    const r = planDeshacer(tocado, inversa, versionesDe(elementos, inversa))
    expect(r.omitidos).toBe(3)
    expect(r.aviso).toBe('3 elementos no se restauraron porque los cambiaste')
  })

  it('un elemento que falta omite su unidad con mensaje singular', () => {
    const { elementos } = aplicarOps([], [agregar('n')])
    const inversa = calcularInversa([], elementos)
    const r = planDeshacer([], inversa, versionesDe(elementos, inversa))
    expect(r.omitidos).toBe(1)
    expect(r.aviso).toBe('1 elemento no se restauró porque lo cambiaste')
  })
})

describe('camposDeshacer', () => {
  it('manda inversa y versiones bajo el tope', () => {
    const { elementos } = aplicarOps([], [agregar('n')])
    const c = camposDeshacer([], elementos)
    expect(c.sin_deshacer).toBe(false)
    expect(c.inversa).toEqual({ restaurar: [], quitar: ['n'] })
    expect(Object.keys(c.versiones)).toEqual(['n'])
  })

  it('sobre 512 KiB manda null y sin_deshacer', () => {
    const grande = el('g', { texto: 'x'.repeat(600 * 1024) })
    const { elementos } = aplicarOps([grande], [{ accion: 'mover', ids: ['g'], dx: 1, dy: 0 }])
    const c = camposDeshacer([grande], elementos)
    expect(c).toEqual({ inversa: null, versiones: null, sin_deshacer: true })
  })
})
