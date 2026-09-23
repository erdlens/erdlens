package introspect

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"sort"
	"strings"

	_ "github.com/go-sql-driver/mysql"

	"github.com/erdlens/erdlens/internal/schema"
)

// MySQL is the database/sql-backed Introspector for MySQL and MariaDB.
type MySQL struct {
	db *sql.DB
}

// NewMySQL opens a MySQL/MariaDB connection and returns a ready Introspector.
// Accepts mysql:// and mariadb:// URLs; rewrites them to the go-sql-driver DSN form.
func NewMySQL(ctx context.Context, dsn string) (*MySQL, error) {
	driverDSN, err := mysqlDriverDSN(dsn)
	if err != nil {
		return nil, err
	}
	db, err := sql.Open("mysql", driverDSN)
	if err != nil {
		return nil, fmt.Errorf("mysql open: %w", err)
	}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("mysql ping: %w", err)
	}
	return &MySQL{db: db}, nil
}

// Close releases the underlying connection pool.
func (m *MySQL) Close() error {
	if m.db == nil {
		return nil
	}
	return m.db.Close()
}

// Introspect reads information_schema into the canonical IR.
func (m *MySQL) Introspect(ctx context.Context, opts Options) (*schema.Schema, error) {
	if len(opts.Schemas) == 0 {
		dbName, err := m.currentDatabase(ctx)
		if err != nil {
			return nil, err
		}
		if dbName == "" {
			return nil, fmt.Errorf("mysql: no database selected; pass --schema or include a database in the DSN")
		}
		opts.Schemas = []string{dbName}
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

	out := &schema.Schema{Dialect: "mysql"}
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

func (m *MySQL) currentDatabase(ctx context.Context) (string, error) {
	var name sql.NullString
	if err := m.db.QueryRowContext(ctx, "SELECT DATABASE()").Scan(&name); err != nil {
		return "", fmt.Errorf("select database: %w", err)
	}
	return name.String, nil
}

func (m *MySQL) readTables(ctx context.Context, opts Options) (map[string]*schema.Table, error) {
	q, args := schemaInQuery(`
SELECT TABLE_SCHEMA, TABLE_NAME, IFNULL(TABLE_COMMENT, '')
FROM information_schema.TABLES
WHERE TABLE_TYPE = 'BASE TABLE'
  AND TABLE_SCHEMA IN (%s)
ORDER BY TABLE_SCHEMA, TABLE_NAME
`, opts.Schemas)
	rows, err := m.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

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

func (m *MySQL) readColumns(ctx context.Context, opts Options, tables map[string]*schema.Table) error {
	q, args := schemaInQuery(`
SELECT
    TABLE_SCHEMA,
    TABLE_NAME,
    COLUMN_NAME,
    COLUMN_TYPE,
    IS_NULLABLE,
    COLUMN_DEFAULT,
    IFNULL(COLUMN_COMMENT, ''),
    ORDINAL_POSITION
FROM information_schema.COLUMNS
WHERE TABLE_SCHEMA IN (%s)
ORDER BY TABLE_SCHEMA, TABLE_NAME, ORDINAL_POSITION
`, opts.Schemas)
	rows, err := m.db.QueryContext(ctx, q, args...)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var nsp, table, name, typ, nullable, comment string
		var def sql.NullString
		var ord int
		if err := rows.Scan(&nsp, &table, &name, &typ, &nullable, &def, &comment, &ord); err != nil {
			return err
		}
		t, ok := tables[tableKey(nsp, table)]
		if !ok {
			continue
		}
		col := schema.Column{
			Name:     name,
			Type:     typ,
			Nullable: strings.EqualFold(nullable, "YES"),
			Comment:  comment,
		}
		if def.Valid {
			col.Default = def.String
		}
		t.Columns = append(t.Columns, col)
	}
	return rows.Err()
}

func (m *MySQL) readPrimaryKeys(ctx context.Context, opts Options, tables map[string]*schema.Table) error {
	q, args := schemaInQuery(`
SELECT
    kcu.TABLE_SCHEMA,
    kcu.TABLE_NAME,
    kcu.COLUMN_NAME,
    kcu.ORDINAL_POSITION
FROM information_schema.TABLE_CONSTRAINTS tc
JOIN information_schema.KEY_COLUMN_USAGE kcu
  ON kcu.CONSTRAINT_SCHEMA = tc.CONSTRAINT_SCHEMA
 AND kcu.CONSTRAINT_NAME = tc.CONSTRAINT_NAME
 AND kcu.TABLE_SCHEMA = tc.TABLE_SCHEMA
 AND kcu.TABLE_NAME = tc.TABLE_NAME
WHERE tc.CONSTRAINT_TYPE = 'PRIMARY KEY'
  AND tc.TABLE_SCHEMA IN (%s)
ORDER BY kcu.TABLE_SCHEMA, kcu.TABLE_NAME, kcu.ORDINAL_POSITION
`, opts.Schemas)
	rows, err := m.db.QueryContext(ctx, q, args...)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
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

func (m *MySQL) readForeignKeys(ctx context.Context, opts Options, tables map[string]*schema.Table) error {
	q, args := schemaInQuery(`
SELECT
    kcu.CONSTRAINT_NAME,
    kcu.TABLE_SCHEMA,
    kcu.TABLE_NAME,
    kcu.COLUMN_NAME,
    kcu.REFERENCED_TABLE_SCHEMA,
    kcu.REFERENCED_TABLE_NAME,
    kcu.REFERENCED_COLUMN_NAME,
    rc.DELETE_RULE,
    rc.UPDATE_RULE,
    kcu.ORDINAL_POSITION
FROM information_schema.KEY_COLUMN_USAGE kcu
JOIN information_schema.REFERENTIAL_CONSTRAINTS rc
  ON rc.CONSTRAINT_SCHEMA = kcu.CONSTRAINT_SCHEMA
 AND rc.CONSTRAINT_NAME = kcu.CONSTRAINT_NAME
WHERE kcu.REFERENCED_TABLE_NAME IS NOT NULL
  AND kcu.TABLE_SCHEMA IN (%s)
ORDER BY kcu.TABLE_SCHEMA, kcu.TABLE_NAME, kcu.CONSTRAINT_NAME, kcu.ORDINAL_POSITION
`, opts.Schemas)
	rows, err := m.db.QueryContext(ctx, q, args...)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()

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
				OnDelete:  mysqlFKAction(delRule),
				OnUpdate:  mysqlFKAction(updRule),
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
// that should be skipped when reading STATISTICS indexes.
func (m *MySQL) readUniqueConstraints(ctx context.Context, opts Options, tables map[string]*schema.Table) (map[string]map[string]bool, error) {
	q, args := schemaInQuery(`
SELECT
    kcu.TABLE_SCHEMA,
    kcu.TABLE_NAME,
    kcu.CONSTRAINT_NAME,
    kcu.COLUMN_NAME,
    kcu.ORDINAL_POSITION
FROM information_schema.TABLE_CONSTRAINTS tc
JOIN information_schema.KEY_COLUMN_USAGE kcu
  ON kcu.CONSTRAINT_SCHEMA = tc.CONSTRAINT_SCHEMA
 AND kcu.CONSTRAINT_NAME = tc.CONSTRAINT_NAME
 AND kcu.TABLE_SCHEMA = tc.TABLE_SCHEMA
 AND kcu.TABLE_NAME = tc.TABLE_NAME
WHERE tc.CONSTRAINT_TYPE = 'UNIQUE'
  AND tc.TABLE_SCHEMA IN (%s)
ORDER BY kcu.TABLE_SCHEMA, kcu.TABLE_NAME, kcu.CONSTRAINT_NAME, kcu.ORDINAL_POSITION
`, opts.Schemas)
	rows, err := m.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	type uKey struct{ table, name string }
	acc := map[uKey]*schema.Index{}
	order := map[string][]uKey{}
	colsByKey := map[uKey][]string{}
	uniqueNames := map[string]map[string]bool{} // schema.table → constraint names

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
			// Drop from multi-col emission.
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

func (m *MySQL) readIndexes(ctx context.Context, opts Options, tables map[string]*schema.Table, uniqueNames map[string]map[string]bool) error {
	q, args := schemaInQuery(`
SELECT
    TABLE_SCHEMA,
    TABLE_NAME,
    INDEX_NAME,
    NON_UNIQUE,
    COLUMN_NAME,
    SEQ_IN_INDEX
FROM information_schema.STATISTICS
WHERE TABLE_SCHEMA IN (%s)
  AND INDEX_NAME != 'PRIMARY'
ORDER BY TABLE_SCHEMA, TABLE_NAME, INDEX_NAME, SEQ_IN_INDEX
`, opts.Schemas)
	rows, err := m.db.QueryContext(ctx, q, args...)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()

	type iKey struct{ table, name string }
	acc := map[iKey]*schema.Index{}
	order := map[string][]iKey{}

	for rows.Next() {
		var nsp, table, name, col string
		var nonUnique, seq int
		if err := rows.Scan(&nsp, &table, &name, &nonUnique, &col, &seq); err != nil {
			return err
		}
		tk := tableKey(nsp, table)
		if _, ok := tables[tk]; !ok {
			continue
		}
		// Skip unique indexes already represented via UNIQUE constraints / Column.Unique.
		if nonUnique == 0 && uniqueNames[tk] != nil && uniqueNames[tk][name] {
			continue
		}
		key := iKey{tk, name}
		idx, ok := acc[key]
		if !ok {
			idx = &schema.Index{Name: name, Unique: nonUnique == 0}
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
			if idx.Unique && uniqueNames[tk] != nil && uniqueNames[tk][idx.Name] {
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

// mysqlFKAction maps information_schema referential rules to IR vocabulary.
func mysqlFKAction(rule string) string {
	switch strings.ToUpper(strings.TrimSpace(rule)) {
	case "", "NO ACTION":
		return ""
	case "RESTRICT":
		return "restrict"
	case "CASCADE":
		return "cascade"
	case "SET NULL":
		return "set_null"
	case "SET DEFAULT":
		return "set_default"
	default:
		return strings.ToLower(strings.ReplaceAll(rule, " ", "_"))
	}
}

// schemaInQuery builds a query with IN ($n…) placeholders for schema names.
func schemaInQuery(format string, schemas []string) (string, []any) {
	ph := make([]string, len(schemas))
	args := make([]any, len(schemas))
	for i, s := range schemas {
		ph[i] = "?"
		args[i] = s
	}
	return fmt.Sprintf(format, strings.Join(ph, ", ")), args
}

// mysqlDriverDSN rewrites mysql:// and mariadb:// URLs into go-sql-driver form.
//
// Accepted inputs:
//   - mysql://user:pass@localhost:3306/dbname
//   - mysql://user:pass@tcp(localhost:3306)/dbname
//   - mariadb://… (same rules)
func mysqlDriverDSN(dsn string) (string, error) {
	lower := strings.ToLower(dsn)
	var rest string
	switch {
	case strings.HasPrefix(lower, "mysql://"):
		rest = dsn[len("mysql://"):]
	case strings.HasPrefix(lower, "mariadb://"):
		rest = dsn[len("mariadb://"):]
	default:
		return "", fmt.Errorf("mysql: unexpected scheme in %q", dsn)
	}

	// Already in go-sql-driver form after scheme strip:
	//   user:pass@tcp(host:port)/db
	if strings.Contains(rest, "@tcp(") || strings.Contains(rest, "@unix(") ||
		strings.HasPrefix(rest, "tcp(") || strings.HasPrefix(rest, "unix(") {
		return rest, nil
	}

	u, err := url.Parse("mysql://" + rest)
	if err != nil {
		return "", fmt.Errorf("parse mysql dsn: %w", err)
	}
	user := ""
	if u.User != nil {
		user = u.User.Username()
		if pass, ok := u.User.Password(); ok {
			user += ":" + pass
		}
	}

	host := u.Host
	if host == "" {
		host = "localhost:3306"
	}
	addr := "tcp(" + host + ")"

	dbName := strings.TrimPrefix(u.Path, "/")
	out := ""
	if user != "" {
		out = user + "@"
	}
	out += addr
	if dbName != "" {
		out += "/" + dbName
	}
	if u.RawQuery != "" {
		out += "?" + u.RawQuery
	}
	return out, nil
}
