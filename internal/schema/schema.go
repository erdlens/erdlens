// Package schema defines the canonical intermediate representation (IR)
// used across introspection, file I/O, and the viewer API.
//
// Every DB driver introspects into these types; the .erd file parser/writer
// serializes to/from them; the HTTP server exposes them as JSON to the viewer.
// Keep this package free of driver- or transport-specific concerns.
package schema

// Schema is the top-level container for an introspected or file-loaded database.
type Schema struct {
	Name     string    `json:"name,omitempty"`
	Dialect  string    `json:"dialect,omitempty"` // "postgres" | "mysql" | "sqlite" | "mssql"
	Views    []View    `json:"views,omitempty"`     // UI filter presets (not SQL VIEW objects)
	Tables   []Table   `json:"tables"`
	SQLViews []SQLView `json:"sql_views,omitempty"` // database VIEW / materialized view relations
}

// View is a saved, named subset of tables that the viewer can filter to.
// Persisted alongside tables in the .erd file so subgraph presets travel
// with the schema.
type View struct {
	Name    string   `json:"name"`
	Include []string `json:"include,omitempty"` // glob patterns; empty means all
	Exclude []string `json:"exclude,omitempty"` // glob patterns; wins over include
}

// SQLView describes a database VIEW or materialized view. Columns only —
// no invented PKs/FKs. Distinct from View (UI filter presets).
type SQLView struct {
	Name         string   `json:"name"`
	Schema       string   `json:"schema,omitempty"`
	Comment      string   `json:"comment,omitempty"`
	Materialized bool     `json:"materialized,omitempty"`
	Columns      []Column `json:"columns"`
	Layout       *Layout  `json:"layout,omitempty"`
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
	RefSchema  string   `json:"ref_schema,omitempty"` // e.g. "public" / "auth" in Postgres
	RefTable   string   `json:"ref_table"`
	RefColumns []string `json:"ref_columns"`
	OnDelete   string   `json:"on_delete,omitempty"`
	OnUpdate   string   `json:"on_update,omitempty"`
}

// IsDefaultSchema reports whether name is a dialect default that should use
// the bare table name for identity/display (Postgres public, MSSQL dbo, or empty).
func IsDefaultSchema(name string) bool {
	return name == "" || name == "public" || name == "dbo"
}

// TableID returns a stable identity for a table. Non-default schemas are
// qualified as "schema.name"; empty, "public", and "dbo" use the bare table
// name so single-schema diagrams stay compatible with existing hashes/layouts.
func TableID(t Table) string {
	if !IsDefaultSchema(t.Schema) {
		return t.Schema + "." + t.Name
	}
	return t.Name
}

// RefTableID returns the identity of the table referenced by fk.
func RefTableID(fk ForeignKey) string {
	if !IsDefaultSchema(fk.RefSchema) {
		return fk.RefSchema + "." + fk.RefTable
	}
	return fk.RefTable
}

// SQLViewID returns a stable identity for a SQL view, using the same
// default-schema rules as TableID.
func SQLViewID(v SQLView) string {
	if !IsDefaultSchema(v.Schema) {
		return v.Schema + "." + v.Name
	}
	return v.Name
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
