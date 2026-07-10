import dagre from '@dagrejs/dagre'
import type { Table, Layout } from './types'

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

  const known = new Set(tables.map((t) => t.name))
  for (const t of tables) {
    g.setNode(t.name, { width: NODE_WIDTH, height: nodeHeight(t) })
  }
  for (const t of tables) {
    for (const fk of t.foreign_keys ?? []) {
      if (known.has(fk.ref_table)) {
        g.setEdge(t.name, fk.ref_table)
      }
    }
  }
  dagre.layout(g)

  const result = new Map<string, Layout>()
  for (const t of tables) {
    const n = g.node(t.name)
    result.set(t.name, { x: n.x - n.width / 2, y: n.y - n.height / 2 })
  }
  return result
}
