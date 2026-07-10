export interface Schema {
  name?: string
  dialect?: string
  views?: View[]
  tables: Table[]
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
