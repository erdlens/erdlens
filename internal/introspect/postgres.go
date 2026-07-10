package introspect

import (
	"context"
	"fmt"
	"sort"

	"github.com/jackc/pgx/v5"

	"github.com/erdlens/erdlens/internal/schema"
)

// Postgres is the pgx-backed Introspector for PostgreSQL.
type Postgres struct {
	conn *pgx.Conn
}

// NewPostgres opens a pgx connection and returns a ready-to-use Introspector.
func NewPostgres(ctx context.Context, dsn string) (*Postgres, error) {
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("pgx connect: %w", err)
	}
	if err := conn.Ping(ctx); err != nil {
		_ = conn.Close(ctx)
		return nil, fmt.Errorf("pgx ping: %w", err)
	}
	return &Postgres{conn: conn}, nil
}

// Close releases the underlying connection.
func (p *Postgres) Close() error {
	if p.conn == nil {
		return nil
	}
	return p.conn.Close(context.Background())
}

// Introspect reads pg_catalog / information_schema into the canonical IR.
// The output is deterministic: tables and their contents are sorted so that
// repeated runs against the same DB produce byte-identical .erd files.
func (p *Postgres) Introspect(ctx context.Context, opts Options) (*schema.Schema, error) {
	if len(opts.Schemas) == 0 {
		opts.Schemas = []string{"public"}
	}

	tables, err := p.readTables(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("read tables: %w", err)
	}
	if err := p.readColumns(ctx, opts, tables); err != nil {
		return nil, fmt.Errorf("read columns: %w", err)
	}
	if err := p.readPrimaryKeys(ctx, opts, tables); err != nil {
		return nil, fmt.Errorf("read primary keys: %w", err)
	}
	if err := p.readForeignKeys(ctx, opts, tables); err != nil {
		return nil, fmt.Errorf("read foreign keys: %w", err)
	}
	if err := p.readUniqueConstraints(ctx, opts, tables); err != nil {
		return nil, fmt.Errorf("read unique constraints: %w", err)
	}
	if err := p.readIndexes(ctx, opts, tables); err != nil {
		return nil, fmt.Errorf("read indexes: %w", err)
	}

	// Flatten map → deterministic slice.
	out := &schema.Schema{Dialect: "postgres"}
	keys := make([]string, 0, len(tables))
	for k := range tables {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		out.Tables = append(out.Tables, *tables[k])
	}
	return out, nil
}

// tableKey uniquely identifies a table within an introspection pass.
func tableKey(schemaName, tableName string) string {
	return schemaName + "." + tableName
}

func (p *Postgres) readTables(ctx context.Context, opts Options) (map[string]*schema.Table, error) {
	const q = `
SELECT n.nspname, c.relname, COALESCE(obj_description(c.oid, 'pg_class'), '')
FROM pg_class c
JOIN pg_namespace n ON n.oid = c.relnamespace
WHERE c.relkind IN ('r', 'p')
  AND n.nspname = ANY($1)
ORDER BY n.nspname, c.relname
`
	rows, err := p.conn.Query(ctx, q, opts.Schemas)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tables := make(map[string]*schema.Table)
	for rows.Next() {
		var nsp, name, comment string
		if err := rows.Scan(&nsp, &name, &comment); err != nil {
			return nil, err
		}
		if !opts.Match(name) {
			continue
		}
		tables[tableKey(nsp, name)] = &schema.Table{
			Name:    name,
			Schema:  nsp,
			Comment: comment,
		}
	}
	return tables, rows.Err()
}

func (p *Postgres) readColumns(ctx context.Context, opts Options, tables map[string]*schema.Table) error {
	const q = `
SELECT
    n.nspname,
    c.relname,
    a.attname,
    format_type(a.atttypid, a.atttypmod),
    NOT a.attnotnull,
    COALESCE(pg_get_expr(ad.adbin, ad.adrelid), ''),
    COALESCE(col_description(c.oid, a.attnum), '')
FROM pg_attribute a
JOIN pg_class c ON c.oid = a.attrelid
JOIN pg_namespace n ON n.oid = c.relnamespace
LEFT JOIN pg_attrdef ad ON ad.adrelid = a.attrelid AND ad.adnum = a.attnum
WHERE c.relkind IN ('r', 'p')
  AND n.nspname = ANY($1)
  AND a.attnum > 0
  AND NOT a.attisdropped
ORDER BY n.nspname, c.relname, a.attnum
`
	rows, err := p.conn.Query(ctx, q, opts.Schemas)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var nsp, table, name, typ, def, comment string
		var nullable bool
		if err := rows.Scan(&nsp, &table, &name, &typ, &nullable, &def, &comment); err != nil {
			return err
		}
		t, ok := tables[tableKey(nsp, table)]
		if !ok {
			continue
		}
		t.Columns = append(t.Columns, schema.Column{
			Name:     name,
			Type:     typ,
			Nullable: nullable,
			Default:  def,
			Comment:  comment,
		})
	}
	return rows.Err()
}

func (p *Postgres) readPrimaryKeys(ctx context.Context, opts Options, tables map[string]*schema.Table) error {
	const q = `
SELECT
    n.nspname,
    c.relname,
    a.attname
FROM pg_constraint con
JOIN pg_class c ON c.oid = con.conrelid
JOIN pg_namespace n ON n.oid = c.relnamespace
JOIN unnest(con.conkey) WITH ORDINALITY AS k(attnum, ord) ON true
JOIN pg_attribute a ON a.attrelid = c.oid AND a.attnum = k.attnum
WHERE con.contype = 'p'
  AND n.nspname = ANY($1)
ORDER BY n.nspname, c.relname, k.ord
`
	rows, err := p.conn.Query(ctx, q, opts.Schemas)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var nsp, table, col string
		if err := rows.Scan(&nsp, &table, &col); err != nil {
			return err
		}
		if t, ok := tables[tableKey(nsp, table)]; ok {
			t.PrimaryKey = append(t.PrimaryKey, col)
		}
	}
	return rows.Err()
}

// fkAction maps a pg_constraint confdeltype/confupdtype code to a lowercase word.
// Returns "" for the default (NO ACTION) so we can omit it from output.
func fkAction(code string) string {
	switch code {
	case "a", "":
		return "" // NO ACTION (default)
	case "r":
		return "restrict"
	case "c":
		return "cascade"
	case "n":
		return "set_null"
	case "d":
		return "set_default"
	default:
		return code
	}
}

func (p *Postgres) readForeignKeys(ctx context.Context, opts Options, tables map[string]*schema.Table) error {
	const q = `
SELECT
    con.conname,
    n.nspname,
    c.relname,
    a.attname,
    fc.relname AS ref_table,
    fa.attname AS ref_column,
    con.confdeltype,
    con.confupdtype,
    k.ord
FROM pg_constraint con
JOIN pg_class c ON c.oid = con.conrelid
JOIN pg_namespace n ON n.oid = c.relnamespace
JOIN pg_class fc ON fc.oid = con.confrelid
JOIN unnest(con.conkey) WITH ORDINALITY AS k(attnum, ord) ON true
JOIN pg_attribute a ON a.attrelid = c.oid AND a.attnum = k.attnum
JOIN unnest(con.confkey) WITH ORDINALITY AS fk(attnum, ord2) ON fk.ord2 = k.ord
JOIN pg_attribute fa ON fa.attrelid = fc.oid AND fa.attnum = fk.attnum
WHERE con.contype = 'f'
  AND n.nspname = ANY($1)
ORDER BY n.nspname, c.relname, con.conname, k.ord
`
	rows, err := p.conn.Query(ctx, q, opts.Schemas)
	if err != nil {
		return err
	}
	defer rows.Close()

	// Accumulate multi-column FKs by (table, constraint name).
	type fkKey struct{ table, name string }
	acc := map[fkKey]*schema.ForeignKey{}
	order := map[string][]fkKey{}

	for rows.Next() {
		var name, nsp, table, col, refTable, refCol, delCode, updCode string
		var ord int
		if err := rows.Scan(&name, &nsp, &table, &col, &refTable, &refCol, &delCode, &updCode, &ord); err != nil {
			return err
		}
		tk := tableKey(nsp, table)
		if _, ok := tables[tk]; !ok {
			continue
		}
		key := fkKey{tk, name}
		fk, ok := acc[key]
		if !ok {
			fk = &schema.ForeignKey{
				Name:     name,
				RefTable: refTable,
				OnDelete: fkAction(delCode),
				OnUpdate: fkAction(updCode),
			}
			acc[key] = fk
			order[tk] = append(order[tk], key)
		}
		fk.Columns = append(fk.Columns, col)
		fk.RefColumns = append(fk.RefColumns, refCol)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for tk, keys := range order {
		t := tables[tk]
		for _, k := range keys {
			t.ForeignKeys = append(t.ForeignKeys, *acc[k])
		}
		sort.SliceStable(t.ForeignKeys, func(i, j int) bool {
			return t.ForeignKeys[i].Name < t.ForeignKeys[j].Name
		})
	}
	return nil
}

func (p *Postgres) readUniqueConstraints(ctx context.Context, opts Options, tables map[string]*schema.Table) error {
	const q = `
SELECT
    n.nspname,
    c.relname,
    con.conname,
    a.attname,
    k.ord,
    array_length(con.conkey, 1)
FROM pg_constraint con
JOIN pg_class c ON c.oid = con.conrelid
JOIN pg_namespace n ON n.oid = c.relnamespace
JOIN unnest(con.conkey) WITH ORDINALITY AS k(attnum, ord) ON true
JOIN pg_attribute a ON a.attrelid = c.oid AND a.attnum = k.attnum
WHERE con.contype = 'u'
  AND n.nspname = ANY($1)
ORDER BY n.nspname, c.relname, con.conname, k.ord
`
	rows, err := p.conn.Query(ctx, q, opts.Schemas)
	if err != nil {
		return err
	}
	defer rows.Close()

	type uKey struct{ table, name string }
	acc := map[uKey]*schema.Index{}
	order := map[string][]uKey{}
	// Track single-column constraints so we can fold them into Column.Unique.
	single := map[string]map[string]bool{}

	for rows.Next() {
		var nsp, table, name, col string
		var ord, arity int
		if err := rows.Scan(&nsp, &table, &name, &col, &ord, &arity); err != nil {
			return err
		}
		tk := tableKey(nsp, table)
		if _, ok := tables[tk]; !ok {
			continue
		}
		if arity == 1 {
			if single[tk] == nil {
				single[tk] = map[string]bool{}
			}
			single[tk][col] = true
			continue
		}
		key := uKey{tk, name}
		idx, ok := acc[key]
		if !ok {
			idx = &schema.Index{Name: name, Unique: true}
			acc[key] = idx
			order[tk] = append(order[tk], key)
		}
		idx.Columns = append(idx.Columns, col)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	// Fold single-column uniques into Column.Unique.
	for tk, cols := range single {
		t := tables[tk]
		for i := range t.Columns {
			if cols[t.Columns[i].Name] {
				t.Columns[i].Unique = true
			}
		}
	}
	// Emit multi-column uniques as unique indexes on the table.
	for tk, keys := range order {
		t := tables[tk]
		for _, k := range keys {
			t.Indexes = append(t.Indexes, *acc[k])
		}
	}
	return nil
}

func (p *Postgres) readIndexes(ctx context.Context, opts Options, tables map[string]*schema.Table) error {
	// Skip primary-key and unique-constraint backing indexes; those are already
	// represented via PrimaryKey / Column.Unique / multi-column unique indexes.
	const q = `
SELECT
    n.nspname,
    c.relname,
    ic.relname AS index_name,
    ix.indisunique,
    a.attname,
    k.ord
FROM pg_index ix
JOIN pg_class ic ON ic.oid = ix.indexrelid
JOIN pg_class c ON c.oid = ix.indrelid
JOIN pg_namespace n ON n.oid = c.relnamespace
JOIN unnest(ix.indkey) WITH ORDINALITY AS k(attnum, ord) ON true
JOIN pg_attribute a ON a.attrelid = c.oid AND a.attnum = k.attnum
WHERE ix.indisprimary = false
  AND NOT EXISTS (
    SELECT 1 FROM pg_constraint con WHERE con.conindid = ic.oid AND con.contype IN ('u','p')
  )
  AND n.nspname = ANY($1)
  AND k.attnum > 0
ORDER BY n.nspname, c.relname, ic.relname, k.ord
`
	rows, err := p.conn.Query(ctx, q, opts.Schemas)
	if err != nil {
		return err
	}
	defer rows.Close()

	type iKey struct{ table, name string }
	acc := map[iKey]*schema.Index{}
	order := map[string][]iKey{}

	for rows.Next() {
		var nsp, table, name, col string
		var unique bool
		var ord int
		if err := rows.Scan(&nsp, &table, &name, &unique, &col, &ord); err != nil {
			return err
		}
		tk := tableKey(nsp, table)
		if _, ok := tables[tk]; !ok {
			continue
		}
		key := iKey{tk, name}
		idx, ok := acc[key]
		if !ok {
			idx = &schema.Index{Name: name, Unique: unique}
			acc[key] = idx
			order[tk] = append(order[tk], key)
		}
		idx.Columns = append(idx.Columns, col)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for tk, keys := range order {
		t := tables[tk]
		for _, k := range keys {
			t.Indexes = append(t.Indexes, *acc[k])
		}
		sort.SliceStable(t.Indexes, func(i, j int) bool {
			return t.Indexes[i].Name < t.Indexes[j].Name
		})
	}
	return nil
}
