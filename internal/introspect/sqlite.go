package introspect

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"sort"
	"strings"

	_ "modernc.org/sqlite"

	"github.com/erdlens/erdlens/internal/schema"
)

// SQLite is the modernc.org/sqlite-backed Introspector.
type SQLite struct {
	db *sql.DB
}

// NewSQLite opens a SQLite database and returns a ready Introspector.
// Accepts sqlite:// and sqlite3:// DSNs (see sqlitePathFromDSN).
func NewSQLite(ctx context.Context, dsn string) (*SQLite, error) {
	path, err := sqlitePathFromDSN(dsn)
	if err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("sqlite open: %w", err)
	}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("sqlite ping: %w", err)
	}
	if _, err := db.ExecContext(ctx, "PRAGMA foreign_keys = ON"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("sqlite enable foreign_keys: %w", err)
	}
	return &SQLite{db: db}, nil
}

// Close releases the underlying connection.
func (s *SQLite) Close() error {
	if s.db == nil {
		return nil
	}
	return s.db.Close()
}

// Introspect reads sqlite_master / PRAGMA metadata into the canonical IR.
// Options.Schemas is ignored (single main catalog). Table.Schema stays empty.
func (s *SQLite) Introspect(ctx context.Context, opts Options) (*schema.Schema, error) {
	tables, err := s.readTables(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("read tables: %w", err)
	}
	if err := s.readColumnsAndPKs(ctx, tables); err != nil {
		return nil, fmt.Errorf("read columns: %w", err)
	}
	if err := s.readForeignKeys(ctx, tables); err != nil {
		return nil, fmt.Errorf("read foreign keys: %w", err)
	}
	if err := s.readIndexes(ctx, tables); err != nil {
		return nil, fmt.Errorf("read indexes: %w", err)
	}

	sqlViews, err := s.readSQLViews(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("read sql views: %w", err)
	}
	if err := s.readSQLViewColumns(ctx, sqlViews); err != nil {
		return nil, fmt.Errorf("read sql view columns: %w", err)
	}

	out := &schema.Schema{Dialect: "sqlite"}
	keys := make([]string, 0, len(tables))
	for k := range tables {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		out.Tables = append(out.Tables, *tables[k])
	}
	vkeys := make([]string, 0, len(sqlViews))
	for k := range sqlViews {
		vkeys = append(vkeys, k)
	}
	sort.Strings(vkeys)
	for _, k := range vkeys {
		out.SQLViews = append(out.SQLViews, *sqlViews[k])
	}
	return out, nil
}

func (s *SQLite) readTables(ctx context.Context, opts Options) (map[string]*schema.Table, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT name FROM sqlite_master
WHERE type = 'table'
  AND name NOT LIKE 'sqlite_%'
ORDER BY name
`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	tables := make(map[string]*schema.Table)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		if !opts.Match(name) {
			continue
		}
		tables[name] = &schema.Table{Name: name}
	}
	return tables, rows.Err()
}

func (s *SQLite) readSQLViews(ctx context.Context, opts Options) (map[string]*schema.SQLView, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT name FROM sqlite_master
WHERE type = 'view'
  AND name NOT LIKE 'sqlite_%'
ORDER BY name
`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	views := make(map[string]*schema.SQLView)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		if !opts.Match(name) {
			continue
		}
		views[name] = &schema.SQLView{Name: name}
	}
	return views, rows.Err()
}

func (s *SQLite) readSQLViewColumns(ctx context.Context, views map[string]*schema.SQLView) error {
	for _, v := range views {
		rows, err := s.db.QueryContext(ctx, fmt.Sprintf("PRAGMA table_info(%s)", quoteIdent(v.Name)))
		if err != nil {
			return fmt.Errorf("table_info %s: %w", v.Name, err)
		}
		for rows.Next() {
			var cid int
			var name, typ string
			var notNull, pk int
			var dflt sql.NullString
			if err := rows.Scan(&cid, &name, &typ, &notNull, &dflt, &pk); err != nil {
				_ = rows.Close()
				return err
			}
			col := schema.Column{
				Name:     name,
				Type:     typ,
				Nullable: notNull == 0,
			}
			if dflt.Valid {
				col.Default = dflt.String
			}
			v.Columns = append(v.Columns, col)
		}
		err = rows.Err()
		_ = rows.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *SQLite) readColumnsAndPKs(ctx context.Context, tables map[string]*schema.Table) error {
	for _, t := range tables {
		rows, err := s.db.QueryContext(ctx, fmt.Sprintf("PRAGMA table_info(%s)", quoteIdent(t.Name)))
		if err != nil {
			return fmt.Errorf("table_info %s: %w", t.Name, err)
		}
		type pkCol struct {
			name string
			ord  int
		}
		var pks []pkCol
		for rows.Next() {
			var cid int
			var name, typ string
			var notNull, pk int
			var def sql.NullString
			if err := rows.Scan(&cid, &name, &typ, &notNull, &def, &pk); err != nil {
				_ = rows.Close()
				return err
			}
			col := schema.Column{
				Name:     name,
				Type:     typ,
				Nullable: notNull == 0 && pk == 0, // PK columns are NOT NULL in SQLite
			}
			if def.Valid {
				col.Default = def.String
			}
			t.Columns = append(t.Columns, col)
			if pk > 0 {
				pks = append(pks, pkCol{name: name, ord: pk})
			}
		}
		err = rows.Err()
		_ = rows.Close()
		if err != nil {
			return err
		}
		sort.Slice(pks, func(i, j int) bool { return pks[i].ord < pks[j].ord })
		for _, p := range pks {
			t.PrimaryKey = append(t.PrimaryKey, p.name)
		}
	}
	return nil
}

func (s *SQLite) readForeignKeys(ctx context.Context, tables map[string]*schema.Table) error {
	for _, t := range tables {
		rows, err := s.db.QueryContext(ctx, fmt.Sprintf("PRAGMA foreign_key_list(%s)", quoteIdent(t.Name)))
		if err != nil {
			return fmt.Errorf("foreign_key_list %s: %w", t.Name, err)
		}

		byID := map[int]*schema.ForeignKey{}
		var order []int

		for rows.Next() {
			var id, seq int
			var table, from, to, onUpdate, onDelete, match string
			// id, seq, table, from, to, on_update, on_delete, match
			if err := rows.Scan(&id, &seq, &table, &from, &to, &onUpdate, &onDelete, &match); err != nil {
				_ = rows.Close()
				return err
			}
			fk, ok := byID[id]
			if !ok {
				fk = &schema.ForeignKey{
					Name:     fmt.Sprintf("fk_%s_%s_%d", t.Name, table, id),
					RefTable: table,
					OnDelete: sqliteFKAction(onDelete),
					OnUpdate: sqliteFKAction(onUpdate),
				}
				byID[id] = fk
				order = append(order, id)
			}
			fk.Columns = append(fk.Columns, from)
			fk.RefColumns = append(fk.RefColumns, to)
		}
		err = rows.Err()
		_ = rows.Close()
		if err != nil {
			return err
		}
		for _, id := range order {
			t.ForeignKeys = append(t.ForeignKeys, *byID[id])
		}
		sort.SliceStable(t.ForeignKeys, func(i, j int) bool {
			return t.ForeignKeys[i].Name < t.ForeignKeys[j].Name
		})
	}
	return nil
}

func (s *SQLite) readIndexes(ctx context.Context, tables map[string]*schema.Table) error {
	for _, t := range tables {
		rows, err := s.db.QueryContext(ctx, fmt.Sprintf("PRAGMA index_list(%s)", quoteIdent(t.Name)))
		if err != nil {
			return fmt.Errorf("index_list %s: %w", t.Name, err)
		}
		type idxMeta struct {
			name   string
			unique bool
			origin string // pk / u / c
		}
		var metas []idxMeta
		for rows.Next() {
			var seq int
			var name string
			var unique int
			var origin string
			var partial int
			// seq, name, unique, origin, partial
			if err := rows.Scan(&seq, &name, &unique, &origin, &partial); err != nil {
				_ = rows.Close()
				return err
			}
			metas = append(metas, idxMeta{name: name, unique: unique != 0, origin: origin})
		}
		err = rows.Err()
		_ = rows.Close()
		if err != nil {
			return err
		}

		for _, meta := range metas {
			if meta.origin == "pk" {
				continue // already in PrimaryKey
			}
			cols, err := s.indexColumns(ctx, meta.name)
			if err != nil {
				return err
			}
			if len(cols) == 0 {
				continue
			}
			if meta.unique && len(cols) == 1 {
				for i := range t.Columns {
					if t.Columns[i].Name == cols[0] {
						t.Columns[i].Unique = true
						break
					}
				}
				continue
			}
			t.Indexes = append(t.Indexes, schema.Index{
				Name:    meta.name,
				Columns: cols,
				Unique:  meta.unique,
			})
		}
		sort.SliceStable(t.Indexes, func(i, j int) bool {
			return t.Indexes[i].Name < t.Indexes[j].Name
		})
	}
	return nil
}

func (s *SQLite) indexColumns(ctx context.Context, indexName string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, fmt.Sprintf("PRAGMA index_info(%s)", quoteIdent(indexName)))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	type colOrd struct {
		ord  int
		name string
	}
	var cols []colOrd
	for rows.Next() {
		var seqno, cid int
		var name sql.NullString
		if err := rows.Scan(&seqno, &cid, &name); err != nil {
			return nil, err
		}
		if !name.Valid || name.String == "" {
			continue // expression index
		}
		cols = append(cols, colOrd{ord: seqno, name: name.String})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.Slice(cols, func(i, j int) bool { return cols[i].ord < cols[j].ord })
	out := make([]string, len(cols))
	for i, c := range cols {
		out[i] = c.name
	}
	return out, nil
}

func sqliteFKAction(rule string) string {
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

// quoteIdent wraps a SQLite identifier in double quotes, escaping embedded quotes.
func quoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

// sqlitePathFromDSN extracts a filesystem path or :memory: from a sqlite:// DSN.
//
// Examples:
//
//	sqlite://:memory:
//	sqlite:///abs/path/db.sqlite   → /abs/path/db.sqlite
//	sqlite://./rel.db              → ./rel.db
//	sqlite3:///tmp/x.db            → /tmp/x.db
func sqlitePathFromDSN(dsn string) (string, error) {
	u, err := url.Parse(dsn)
	if err != nil {
		return "", fmt.Errorf("parse sqlite dsn: %w", err)
	}
	switch strings.ToLower(u.Scheme) {
	case "sqlite", "sqlite3":
	default:
		return "", fmt.Errorf("sqlite: unexpected scheme %q", u.Scheme)
	}

	// sqlite://:memory: — Host is empty / ":memory:", Path may be empty.
	if u.Host == ":memory:" || (u.Host == "" && (u.Path == ":memory:" || u.Opaque == ":memory:")) {
		return ":memory:", nil
	}
	if u.Opaque == ":memory:" {
		return ":memory:", nil
	}

	// Absolute: sqlite:///tmp/db → Path=/tmp/db, Host empty
	// Relative with host-as-path: sqlite://./foo.db → Host=., Path=/foo.db (odd)
	// Relative: sqlite:rel.db is Opaque — rare

	if u.Opaque != "" && u.Path == "" && u.Host == "" {
		return u.Opaque, nil
	}

	path := u.Path
	if u.Host != "" && u.Host != "localhost" {
		// sqlite://./rel.db → Host="." Path="/rel.db"
		// sqlite://rel.db → Host="rel.db" Path=""
		if path == "" {
			path = u.Host
		} else {
			path = u.Host + path
		}
	}
	if path == "" {
		return "", fmt.Errorf("sqlite: empty path in dsn %q", dsn)
	}
	// url.Parse leaves Path with leading / for absolute URLs: sqlite:///tmp/x → /tmp/x
	if u.RawQuery != "" {
		path += "?" + u.RawQuery
	}
	return path, nil
}
