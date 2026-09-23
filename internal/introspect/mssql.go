package introspect

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strconv"
	"strings"

	_ "github.com/microsoft/go-mssqldb"

	"github.com/erdlens/erdlens/internal/schema"
)

// MSSQL is the database/sql-backed Introspector for Microsoft SQL Server.
type MSSQL struct {
	db *sql.DB
}

// NewMSSQL opens a SQL Server connection and returns a ready Introspector.
// Accepts mssql:// and sqlserver:// URLs; rewrites mssql:// to sqlserver://
// for the microsoft/go-mssqldb driver.
func NewMSSQL(ctx context.Context, dsn string) (*MSSQL, error) {
	driverDSN, err := mssqlDriverDSN(dsn)
	if err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlserver", driverDSN)
	if err != nil {
		return nil, fmt.Errorf("mssql open: %w", err)
	}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("mssql ping: %w", err)
	}
	return &MSSQL{db: db}, nil
}

// Close releases the underlying connection pool.
func (m *MSSQL) Close() error {
	if m.db == nil {
		return nil
	}
	return m.db.Close()
}

// Introspect reads information_schema / sys catalogs into the canonical IR.
func (m *MSSQL) Introspect(ctx context.Context, opts Options) (*schema.Schema, error) {
	if len(opts.Schemas) == 0 {
		opts.Schemas = []string{"dbo"}
	}

	tables, err := m.readTables(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("read tables: %w", err)
	}
	if err := m.readColumns(ctx, opts, tables); err != nil {
		return nil, fmt.Errorf("read columns: %w", err)
	}
	if err := m.readPrimaryKeys(ctx, opts, tables); err != nil {
		return nil, fmt.Errorf("read primary keys: %w", err)
	}
	if err := m.readForeignKeys(ctx, opts, tables); err != nil {
		return nil, fmt.Errorf("read foreign keys: %w", err)
	}
	uniqueNames, err := m.readUniqueConstraints(ctx, opts, tables)
	if err != nil {
		return nil, fmt.Errorf("read unique constraints: %w", err)
	}
	if err := m.readIndexes(ctx, opts, tables, uniqueNames); err != nil {
		return nil, fmt.Errorf("read indexes: %w", err)
	}

	out := &schema.Schema{Dialect: "mssql"}
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

func (m *MSSQL) readTables(ctx context.Context, opts Options) (map[string]*schema.Table, error) {
	q, args := mssqlSchemaInQuery(`
SELECT
    s.name,
    t.name,
    ISNULL(CAST(ep.value AS nvarchar(4000)), '')
FROM sys.tables t
JOIN sys.schemas s ON s.schema_id = t.schema_id
LEFT JOIN sys.extended_properties ep
  ON ep.major_id = t.object_id
 AND ep.minor_id = 0
 AND ep.class = 1
 AND ep.name = N'MS_Description'
WHERE t.is_ms_shipped = 0
  AND s.name IN (%s)
ORDER BY s.name, t.name
`, opts.Schemas)
	rows, err := m.db.QueryContext(ctx, q, args...)
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

func (m *MSSQL) readColumns(ctx context.Context, opts Options, tables map[string]*schema.Table) error {
	q, args := mssqlSchemaInQuery(`
SELECT
    s.name,
    t.name,
    c.name,
    ty.name,
    c.max_length,
    c.precision,
    c.scale,
    c.is_nullable,
    ISNULL(dc.definition, ''),
    ISNULL(CAST(ep.value AS nvarchar(4000)), ''),
    c.column_id
FROM sys.columns c
JOIN sys.tables t ON t.object_id = c.object_id
JOIN sys.schemas s ON s.schema_id = t.schema_id
JOIN sys.types ty ON ty.user_type_id = c.user_type_id
LEFT JOIN sys.default_constraints dc
  ON dc.parent_object_id = c.object_id
 AND dc.parent_column_id = c.column_id
LEFT JOIN sys.extended_properties ep
  ON ep.major_id = c.object_id
 AND ep.minor_id = c.column_id
 AND ep.class = 1
 AND ep.name = N'MS_Description'
WHERE t.is_ms_shipped = 0
  AND s.name IN (%s)
ORDER BY s.name, t.name, c.column_id
`, opts.Schemas)
	rows, err := m.db.QueryContext(ctx, q, args...)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var nsp, table, name, typeName, def, comment string
		var maxLen int
		var precision, scale uint8
		var nullable bool
		var colID int
		if err := rows.Scan(&nsp, &table, &name, &typeName, &maxLen, &precision, &scale, &nullable, &def, &comment, &colID); err != nil {
			return err
		}
		t, ok := tables[tableKey(nsp, table)]
		if !ok {
			continue
		}
		col := schema.Column{
			Name:     name,
			Type:     mssqlFormatType(typeName, maxLen, int(precision), int(scale)),
			Nullable: nullable,
			Comment:  comment,
		}
		if def != "" {
			col.Default = def
		}
		t.Columns = append(t.Columns, col)
	}
	return rows.Err()
}

func (m *MSSQL) readPrimaryKeys(ctx context.Context, opts Options, tables map[string]*schema.Table) error {
	q, args := mssqlSchemaInQuery(`
SELECT
    s.name,
    t.name,
    c.name,
    ic.key_ordinal
FROM sys.indexes i
JOIN sys.index_columns ic
  ON ic.object_id = i.object_id
 AND ic.index_id = i.index_id
JOIN sys.columns c
  ON c.object_id = ic.object_id
 AND c.column_id = ic.column_id
JOIN sys.tables t ON t.object_id = i.object_id
JOIN sys.schemas s ON s.schema_id = t.schema_id
WHERE i.is_primary_key = 1
  AND t.is_ms_shipped = 0
  AND s.name IN (%s)
ORDER BY s.name, t.name, ic.key_ordinal
`, opts.Schemas)
	rows, err := m.db.QueryContext(ctx, q, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var nsp, table, col string
		var ord int
		if err := rows.Scan(&nsp, &table, &col, &ord); err != nil {
			return err
		}
		if t, ok := tables[tableKey(nsp, table)]; ok {
			t.PrimaryKey = append(t.PrimaryKey, col)
		}
	}
	return rows.Err()
}

func (m *MSSQL) readForeignKeys(ctx context.Context, opts Options, tables map[string]*schema.Table) error {
	q, args := mssqlSchemaInQuery(`
SELECT
    fk.name,
    SCHEMA_NAME(t.schema_id),
    t.name,
    c.name,
    SCHEMA_NAME(rt.schema_id),
    rt.name,
    rc.name,
    fk.delete_referential_action_desc,
    fk.update_referential_action_desc,
    fkc.constraint_column_id
FROM sys.foreign_keys fk
JOIN sys.foreign_key_columns fkc ON fkc.constraint_object_id = fk.object_id
JOIN sys.tables t ON t.object_id = fk.parent_object_id
JOIN sys.columns c
  ON c.object_id = fkc.parent_object_id
 AND c.column_id = fkc.parent_column_id
JOIN sys.tables rt ON rt.object_id = fk.referenced_object_id
JOIN sys.columns rc
  ON rc.object_id = fkc.referenced_object_id
 AND rc.column_id = fkc.referenced_column_id
WHERE t.is_ms_shipped = 0
  AND SCHEMA_NAME(t.schema_id) IN (%s)
ORDER BY SCHEMA_NAME(t.schema_id), t.name, fk.name, fkc.constraint_column_id
`, opts.Schemas)
	rows, err := m.db.QueryContext(ctx, q, args...)
	if err != nil {
		return err
	}
	defer rows.Close()

	type fkKey struct{ table, name string }
	acc := map[fkKey]*schema.ForeignKey{}
	order := map[string][]fkKey{}

	for rows.Next() {
		var name, nsp, table, col, refSchema, refTable, refCol, delRule, updRule string
		var ord int
		if err := rows.Scan(&name, &nsp, &table, &col, &refSchema, &refTable, &refCol, &delRule, &updRule, &ord); err != nil {
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
				Name:      name,
				RefSchema: refSchema,
				RefTable:  refTable,
				OnDelete:  mssqlFKAction(delRule),
				OnUpdate:  mssqlFKAction(updRule),
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

// readUniqueConstraints folds UNIQUE constraints. Returns constraint/index names
// that should be skipped when reading sys.indexes.
func (m *MSSQL) readUniqueConstraints(ctx context.Context, opts Options, tables map[string]*schema.Table) (map[string]map[string]bool, error) {
	q, args := mssqlSchemaInQuery(`
SELECT
    s.name,
    t.name,
    i.name,
    c.name,
    ic.key_ordinal
FROM sys.indexes i
JOIN sys.index_columns ic
  ON ic.object_id = i.object_id
 AND ic.index_id = i.index_id
JOIN sys.columns c
  ON c.object_id = ic.object_id
 AND c.column_id = ic.column_id
JOIN sys.tables t ON t.object_id = i.object_id
JOIN sys.schemas s ON s.schema_id = t.schema_id
WHERE i.is_unique_constraint = 1
  AND ic.is_included_column = 0
  AND t.is_ms_shipped = 0
  AND s.name IN (%s)
ORDER BY s.name, t.name, i.name, ic.key_ordinal
`, opts.Schemas)
	rows, err := m.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type uKey struct{ table, name string }
	acc := map[uKey]*schema.Index{}
	order := map[string][]uKey{}
	colsByKey := map[uKey][]string{}
	uniqueNames := map[string]map[string]bool{}

	for rows.Next() {
		var nsp, table, name, col string
		var ord int
		if err := rows.Scan(&nsp, &table, &name, &col, &ord); err != nil {
			return nil, err
		}
		tk := tableKey(nsp, table)
		if _, ok := tables[tk]; !ok {
			continue
		}
		key := uKey{tk, name}
		colsByKey[key] = append(colsByKey[key], col)
		if uniqueNames[tk] == nil {
			uniqueNames[tk] = map[string]bool{}
		}
		uniqueNames[tk][name] = true
		if _, ok := acc[key]; !ok {
			acc[key] = &schema.Index{Name: name, Unique: true}
			order[tk] = append(order[tk], key)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	single := map[string]map[string]bool{}
	for key, cols := range colsByKey {
		if len(cols) == 1 {
			if single[key.table] == nil {
				single[key.table] = map[string]bool{}
			}
			single[key.table][cols[0]] = true
			delete(acc, key)
			continue
		}
		acc[key].Columns = cols
	}

	for tk, cols := range single {
		t := tables[tk]
		for i := range t.Columns {
			if cols[t.Columns[i].Name] {
				t.Columns[i].Unique = true
			}
		}
	}
	for tk, keys := range order {
		t := tables[tk]
		for _, k := range keys {
			if idx, ok := acc[k]; ok {
				t.Indexes = append(t.Indexes, *idx)
			}
		}
	}
	return uniqueNames, nil
}

func (m *MSSQL) readIndexes(ctx context.Context, opts Options, tables map[string]*schema.Table, uniqueNames map[string]map[string]bool) error {
	q, args := mssqlSchemaInQuery(`
SELECT
    s.name,
    t.name,
    i.name,
    i.is_unique,
    c.name,
    ic.key_ordinal
FROM sys.indexes i
JOIN sys.index_columns ic
  ON ic.object_id = i.object_id
 AND ic.index_id = i.index_id
JOIN sys.columns c
  ON c.object_id = ic.object_id
 AND c.column_id = ic.column_id
JOIN sys.tables t ON t.object_id = i.object_id
JOIN sys.schemas s ON s.schema_id = t.schema_id
WHERE i.type > 0
  AND i.is_primary_key = 0
  AND i.is_unique_constraint = 0
  AND ic.is_included_column = 0
  AND t.is_ms_shipped = 0
  AND s.name IN (%s)
ORDER BY s.name, t.name, i.name, ic.key_ordinal
`, opts.Schemas)
	rows, err := m.db.QueryContext(ctx, q, args...)
	if err != nil {
		return err
	}
	defer rows.Close()

	type iKey struct{ table, name string }
	acc := map[iKey]*schema.Index{}
	order := map[string][]iKey{}

	for rows.Next() {
		var nsp, table, name, col string
		var isUnique bool
		var ord int
		if err := rows.Scan(&nsp, &table, &name, &isUnique, &col, &ord); err != nil {
			return err
		}
		tk := tableKey(nsp, table)
		if _, ok := tables[tk]; !ok {
			continue
		}
		if isUnique && uniqueNames[tk] != nil && uniqueNames[tk][name] {
			continue
		}
		key := iKey{tk, name}
		idx, ok := acc[key]
		if !ok {
			idx = &schema.Index{Name: name, Unique: isUnique}
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
			idx := acc[k]
			if idx.Unique && len(idx.Columns) == 1 {
				for i := range t.Columns {
					if t.Columns[i].Name == idx.Columns[0] {
						t.Columns[i].Unique = true
						break
					}
				}
				continue
			}
			t.Indexes = append(t.Indexes, *idx)
		}
		sort.SliceStable(t.Indexes, func(i, j int) bool {
			return t.Indexes[i].Name < t.Indexes[j].Name
		})
	}
	return nil
}

// mssqlFKAction maps sys.foreign_keys action descriptions to IR vocabulary.
func mssqlFKAction(rule string) string {
	switch strings.ToUpper(strings.TrimSpace(rule)) {
	case "", "NO_ACTION":
		return ""
	case "CASCADE":
		return "cascade"
	case "SET_NULL":
		return "set_null"
	case "SET_DEFAULT":
		return "set_default"
	default:
		return strings.ToLower(strings.ReplaceAll(rule, " ", "_"))
	}
}

// mssqlFormatType builds a display type string from sys.types / sys.columns fields.
func mssqlFormatType(name string, maxLen, precision, scale int) string {
	lower := strings.ToLower(name)
	switch lower {
	case "varchar", "nvarchar", "char", "nchar", "binary", "varbinary":
		if maxLen < 0 || maxLen == -1 {
			return lower + "(max)"
		}
		// nvarchar/nchar store max_length in bytes (2 per char).
		chars := maxLen
		if lower == "nvarchar" || lower == "nchar" {
			chars = maxLen / 2
		}
		return fmt.Sprintf("%s(%d)", lower, chars)
	case "decimal", "numeric":
		return fmt.Sprintf("%s(%d,%d)", lower, precision, scale)
	case "datetime2", "datetimeoffset", "time":
		return fmt.Sprintf("%s(%d)", lower, scale)
	case "float":
		if precision > 0 {
			return fmt.Sprintf("%s(%d)", lower, precision)
		}
		return lower
	default:
		return lower
	}
}

// mssqlSchemaInQuery builds a query with IN (@p1…) placeholders for schema names.
func mssqlSchemaInQuery(format string, schemas []string) (string, []any) {
	ph := make([]string, len(schemas))
	args := make([]any, len(schemas))
	for i, s := range schemas {
		ph[i] = "@p" + strconv.Itoa(i+1)
		args[i] = s
	}
	return fmt.Sprintf(format, strings.Join(ph, ", ")), args
}

// mssqlDriverDSN rewrites mssql:// URLs to sqlserver:// for go-mssqldb.
// sqlserver:// URLs are passed through unchanged.
func mssqlDriverDSN(dsn string) (string, error) {
	lower := strings.ToLower(dsn)
	switch {
	case strings.HasPrefix(lower, "sqlserver://"):
		return dsn, nil
	case strings.HasPrefix(lower, "mssql://"):
		return "sqlserver://" + dsn[len("mssql://"):], nil
	default:
		return "", fmt.Errorf("mssql: unexpected scheme in %q", dsn)
	}
}
