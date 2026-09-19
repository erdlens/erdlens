import type { ForeignKey, Table } from './types'

/** Empty and "public" share the bare-name identity for single-schema diagrams. */
export function isDefaultSchema(schema?: string): boolean {
  return !schema || schema === 'public'
}

/** Stable node/layout identity: `schema.name` when non-public, else bare name. */
export function tableId(t: Pick<Table, 'name' | 'schema'>): string {
  if (isDefaultSchema(t.schema)) return t.name
  return `${t.schema}.${t.name}`
}

/** Identity of the table referenced by an FK. */
export function refTableId(fk: ForeignKey): string {
  if (isDefaultSchema(fk.ref_schema)) return fk.ref_table
  return `${fk.ref_schema}.${fk.ref_table}`
}

/** Display label: qualify when schema is non-public. */
export function tableLabel(t: Pick<Table, 'name' | 'schema'>): string {
  return tableId(t)
}

/** Normalized schema name for filtering (empty → public). */
export function schemaName(t: Pick<Table, 'schema'>): string {
  return t.schema || 'public'
}

/** Distinct schema names present in tables, sorted. */
export function distinctSchemas(tables: Table[]): string[] {
  const set = new Set<string>()
  for (const t of tables) set.add(schemaName(t))
  return [...set].sort()
}
