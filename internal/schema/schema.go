// Package schema defines the canonical intermediate representation (IR)
// used across introspection, file I/O, and the viewer API.
//
// Every DB driver introspects into these types; the .erd file parser/writer
// serializes to/from them; the HTTP server exposes them as JSON to the viewer.
// Keep this package free of driver- or transport-specific concerns.
package schema

// Schema is the top-level container for an introspected or file-loaded database.
type Schema struct {
	Name    string  `json:"name,omitempty"`
	Dialect string  `json:"dialect,omitempty"` // "postgres" | "mysql" | "sqlite" | "mssql"
	Views   []View  `json:"views,omitempty"`
	Tables  []Table `json:"tables"`
}

// View is a saved, named subset of tables that the viewer can filter to.
// Persisted alongside tables in the .erd file so subgraph presets travel
// with the schema.
type View struct {
	Name    string   `json:"name"`
	Include []string `json:"include,omitempty"` // glob patterns; empty means all
	Exclude []string `json:"exclude,omitempty"` // glob patterns; wins over include
}

// Table describes a relation with its columns, keys, and optional layout hints.
type Table struct {
	Name        string       `json:"name"`
	Schema      string       `json:"schema,omitempty"` // e.g. "public" in Postgres
	Comment     string       `json:"comment,omitempty"`
	Columns     []Column     `json:"columns"`
	PrimaryKey  []string     `json:"primary_key,omitempty"`
	ForeignKeys []ForeignKey `json:"foreign_keys,omitempty"`
	Indexes     []Index      `json:"indexes,omitempty"`
	Layout      *Layout      `json:"layout,omitempty"`
}

// Column describes a single column within a table.
type Column struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Nullable bool   `json:"nullable"`
	Default  string `json:"default,omitempty"`
	Comment  string `json:"comment,omitempty"`
	Unique   bool   `json:"unique,omitempty"`
}

// ForeignKey describes a referential constraint between two tables.
type ForeignKey struct {
	Name       string   `json:"name,omitempty"`
	Columns    []string `json:"columns"`
	RefTable   string   `json:"ref_table"`
	RefColumns []string `json:"ref_columns"`
	OnDelete   string   `json:"on_delete,omitempty"`
	OnUpdate   string   `json:"on_update,omitempty"`
}

// Index describes a non-PK index. Kept minimal for v1.
type Index struct {
	Name    string   `json:"name"`
	Columns []string `json:"columns"`
	Unique  bool     `json:"unique,omitempty"`
}

// Layout is an optional per-table hint persisted to the .erd file so the
// viewer can restore user-arranged positions across sessions.
type Layout struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}
