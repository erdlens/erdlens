export interface Schema {
  name?: string
  dialect?: string
  views?: View[]
  tables: Table[]
  sql_views?: SQLView[]
}

export interface View {
  name: string
  include?: string[]
  exclude?: string[]
}

export interface Table {
  name: string
  schema?: string
  comment?: string
  columns: Column[]
  primary_key?: string[]
  foreign_keys?: ForeignKey[]
  indexes?: Index[]
  layout?: Layout
}

/** Database VIEW or materialized view (not a UI filter preset). */
export interface SQLView {
  name: string
  schema?: string
  comment?: string
  materialized?: boolean
  columns: Column[]
  layout?: Layout
}

/** Canvas relation: table or SQL view (shared identity / layout fields). */
export type Relation = Pick<Table, 'name' | 'schema' | 'comment' | 'columns' | 'layout'> & {
  primary_key?: string[]
  foreign_keys?: ForeignKey[]
  kind: 'table' | 'sql_view'
  materialized?: boolean
}

export interface Column {
  name: string
  type: string
  nullable: boolean
  default?: string
  comment?: string
  unique?: boolean
}

export interface ForeignKey {
  name?: string
  columns: string[]
  ref_schema?: string
  ref_table: string
  ref_columns: string[]
  on_delete?: string
  on_update?: string
}

export interface Index {
  name: string
  columns: string[]
  unique?: boolean
}

export interface Layout {
  x: number
  y: number
}
