import dagre from '@dagrejs/dagre'
import type { Table, Layout } from './types'
import { refTableId, tableId } from './tableId'

export const NODE_WIDTH = 260
const HEADER_H = 34
const ROW_H = 22
const PADDING = 8

export function nodeHeight(table: Table): number {
  return HEADER_H + Math.max(1, table.columns.length) * ROW_H + PADDING
}

// autoLayout computes positions for tables using dagre's rank-based algorithm.
// Coordinates are top-left (Svelte Flow convention).
export function autoLayout(
  tables: Table[],
  opts?: { nodesep?: number; ranksep?: number },
): Map<string, Layout> {
  const g = new dagre.graphlib.Graph()
  g.setGraph({
    rankdir: 'LR',
    nodesep: opts?.nodesep ?? 40,
    ranksep: opts?.ranksep ?? 100,
    marginx: 40,
    marginy: 40,
  })
  g.setDefaultEdgeLabel(() => ({}))

  const known = new Set(tables.map(tableId))
  for (const t of tables) {
    g.setNode(tableId(t), { width: NODE_WIDTH, height: nodeHeight(t) })
  }
  for (const t of tables) {
    const src = tableId(t)
    for (const fk of t.foreign_keys ?? []) {
      const target = refTableId(fk)
      if (known.has(target)) {
        g.setEdge(src, target)
      }
    }
  }
  dagre.layout(g)

  const result = new Map<string, Layout>()
  for (const t of tables) {
    const id = tableId(t)
    const n = g.node(id)
    result.set(id, { x: n.x - n.width / 2, y: n.y - n.height / 2 })
  }
  return result
}

/** Layout for isolate mode: selected table at origin, neighbors spaced by dagre. */
export function isolatedLayout(
  tables: Table[],
  selected: string,
): Map<string, Layout> {
  if (tables.length === 0) return new Map()

  const maxH = Math.max(...tables.map(nodeHeight))
  // Dagre accounts for node bounding boxes; add extra gap for tall tables.
  const gap = Math.max(60, maxH > 400 ? 80 : 40)

  const positions = autoLayout(tables, { nodesep: gap, ranksep: gap + 40 })

  const selPos = positions.get(selected)
  if (!selPos) return positions

  const centered = new Map<string, Layout>()
  for (const [name, pos] of positions) {
    centered.set(name, { x: pos.x - selPos.x, y: pos.y - selPos.y })
  }
  return centered
}
