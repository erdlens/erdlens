import type { ForeignKey, Relation, Schema, SQLView, Table } from './types'

/** Empty, "public", and "dbo" share the bare-name identity for single-schema diagrams. */
export function isDefaultSchema(schema?: string): boolean {
  return !schema || schema === 'public' || schema === 'dbo'
}

/** Stable node/layout identity: `schema.name` when non-default, else bare name. */
export function tableId(t: Pick<Table, 'name' | 'schema'>): string {
  if (isDefaultSchema(t.schema)) return t.name
  return `${t.schema}.${t.name}`
}

/** Identity of the table referenced by an FK. */
export function refTableId(fk: ForeignKey): string {
  if (isDefaultSchema(fk.ref_schema)) return fk.ref_table
  return `${fk.ref_schema}.${fk.ref_table}`
}

/** Display label: qualify when schema is non-default. */
export function tableLabel(t: Pick<Table, 'name' | 'schema'>): string {
  return tableId(t)
}

/** Normalized schema name for filtering (empty → public). */
export function schemaName(t: Pick<Table, 'schema'>): string {
  return t.schema || 'public'
}

/** Distinct schema names present in tables and SQL views, sorted. */
export function distinctSchemas(relations: Pick<Table, 'schema'>[]): string[] {
  const set = new Set<string>()
  for (const t of relations) set.add(schemaName(t))
  return [...set].sort()
}

/** All canvas relations (tables then SQL views) for a schema. */
export function allRelations(schema: Schema): Relation[] {
  const tables: Relation[] = schema.tables.map((t) => ({
    ...t,
    kind: 'table' as const,
  }))
  const views: Relation[] = (schema.sql_views ?? []).map((v: SQLView) => ({
    name: v.name,
    schema: v.schema,
    comment: v.comment,
    columns: v.columns,
    layout: v.layout,
    kind: 'sql_view' as const,
    materialized: v.materialized,
  }))
  return [...tables, ...views]
}
