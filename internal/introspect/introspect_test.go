package introspect

import (
	"context"
	"errors"
	"os"
	"testing"
)

func TestOptionsMatch(t *testing.T) {
	cases := []struct {
		name  string
		opts  Options
		table string
		want  bool
	}{
		{"no filters accepts all", Options{}, "users", true},
		{"include match", Options{Include: []string{"user*"}}, "users", true},
		{"include no match", Options{Include: []string{"user*"}}, "orders", false},
		{"exclude wins over include", Options{Include: []string{"*"}, Exclude: []string{"audit_*"}}, "audit_log", false},
		{"exclude miss", Options{Exclude: []string{"audit_*"}}, "users", true},
		{"multiple includes, one matches", Options{Include: []string{"user*", "order*"}}, "orders", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.opts.Match(tc.table); got != tc.want {
				t.Fatalf("Match(%q) = %v, want %v", tc.table, got, tc.want)
			}
		})
	}
}

func TestOpenUnsupportedDialect(t *testing.T) {
	_, err := Open(context.TODO(), "oracle://foo/bar")
	if err == nil {
		t.Fatal("expected error for unsupported dialect")
	}
	if !errors.Is(err, ErrUnsupportedDialect) {
		t.Fatalf("expected ErrUnsupportedDialect, got %v", err)
	}
}

func TestOpenSupportedSchemes(t *testing.T) {
	// Scheme selection happens before connect; invalid host still proves routing.
	cases := []struct {
		dsn     string
		wantErr string // substring; empty = may fail on connect but not unsupported
	}{
		{"mysql://user:pass@127.0.0.1:1/db", ""},
		{"mariadb://user:pass@127.0.0.1:1/db", ""},
		{"sqlite://:memory:", ""},
		{"sqlite3://:memory:", ""},
		{"postgres://user:pass@127.0.0.1:1/db", ""},
		{"mssql://user:pass@127.0.0.1:1?database=db", ""},
		{"sqlserver://user:pass@127.0.0.1:1?database=db", ""},
	}
	for _, tc := range cases {
		t.Run(tc.dsn, func(t *testing.T) {
			i, err := Open(context.Background(), tc.dsn)
			if err != nil {
				if errors.Is(err, ErrUnsupportedDialect) {
					t.Fatalf("scheme should be supported: %v", err)
				}
				// Connect failure is OK for unreachable hosts.
				return
			}
		defer func() { _ = i.Close() }()
		})
	}
}

func TestOpenMySQLTCPForm(t *testing.T) {
	// url.Parse rejects tcp(host:port); Open must still route by scheme.
	_, err := Open(context.Background(), "mysql://user:p@tcp(127.0.0.1:1)/x")
	if errors.Is(err, ErrUnsupportedDialect) {
		t.Fatalf("tcp() DSN should be supported: %v", err)
	}
}

func TestMySQLDriverDSN(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{
			"mysql://user:pass@localhost:3306/mydb",
			"user:pass@tcp(localhost:3306)/mydb",
		},
		{
			"mariadb://root@127.0.0.1:3307/app?parseTime=true",
			"root@tcp(127.0.0.1:3307)/app?parseTime=true",
		},
		{
			"mysql://user:p@tcp(db:3306)/x",
			"user:p@tcp(db:3306)/x",
		},
	}
	for _, tc := range cases {
		got, err := mysqlDriverDSN(tc.in)
		if err != nil {
			t.Fatalf("mysqlDriverDSN(%q): %v", tc.in, err)
		}
		if got != tc.want {
			t.Fatalf("mysqlDriverDSN(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestSQLitePathFromDSN(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"sqlite://:memory:", ":memory:"},
		{"sqlite3://:memory:", ":memory:"},
		{"sqlite:///tmp/test.db", "/tmp/test.db"},
		{"sqlite://./rel.db", "./rel.db"},
	}
	for _, tc := range cases {
		got, err := sqlitePathFromDSN(tc.in)
		if err != nil {
			t.Fatalf("sqlitePathFromDSN(%q): %v", tc.in, err)
		}
		if got != tc.want {
			t.Fatalf("sqlitePathFromDSN(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestSQLiteIntrospect(t *testing.T) {
	ctx := context.Background()
	i, err := Open(ctx, "sqlite://:memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = i.Close() }()

	// Seed via the concrete type.
	sq := i.(*SQLite)
	_, err = sq.db.ExecContext(ctx, `
CREATE TABLE users (
  id INTEGER PRIMARY KEY,
  email TEXT NOT NULL UNIQUE
);
CREATE TABLE orders (
  id INTEGER PRIMARY KEY,
  user_id INTEGER NOT NULL,
  total INTEGER NOT NULL DEFAULT 0,
  FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);
CREATE INDEX idx_orders_user ON orders(user_id);
CREATE UNIQUE INDEX orders_ext_unique ON orders(id, user_id);
`)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	s, err := i.Introspect(ctx, Options{})
	if err != nil {
		t.Fatalf("introspect: %v", err)
	}
	if s.Dialect != "sqlite" {
		t.Fatalf("dialect = %q", s.Dialect)
	}
	if len(s.Tables) != 2 {
		t.Fatalf("tables = %d, want 2", len(s.Tables))
	}

	byName := map[string]int{}
	for i, t := range s.Tables {
		byName[t.Name] = i
	}
	users := s.Tables[byName["users"]]
	orders := s.Tables[byName["orders"]]

	if users.Schema != "" {
		t.Fatalf("sqlite users.Schema = %q, want empty", users.Schema)
	}
	if len(users.PrimaryKey) != 1 || users.PrimaryKey[0] != "id" {
		t.Fatalf("users PK = %#v", users.PrimaryKey)
	}
	emailUnique := false
	for _, c := range users.Columns {
		if c.Name == "email" && c.Unique {
			emailUnique = true
		}
	}
	if !emailUnique {
		t.Fatal("expected email Column.Unique")
	}

	if len(orders.ForeignKeys) != 1 {
		t.Fatalf("orders FKs = %d", len(orders.ForeignKeys))
	}
	fk := orders.ForeignKeys[0]
	if fk.RefTable != "users" || fk.OnDelete != "cascade" {
		t.Fatalf("FK = %#v", fk)
	}
	if len(fk.Columns) != 1 || fk.Columns[0] != "user_id" {
		t.Fatalf("FK columns = %#v", fk.Columns)
	}

	// Non-unique index present; multi-col unique as Index.
	foundIdx := false
	foundUQ := false
	for _, idx := range orders.Indexes {
		if idx.Name == "idx_orders_user" && !idx.Unique {
			foundIdx = true
		}
		if idx.Unique && len(idx.Columns) == 2 {
			foundUQ = true
		}
	}
	if !foundIdx {
		t.Fatalf("missing idx_orders_user in %#v", orders.Indexes)
	}
	if !foundUQ {
		t.Fatalf("missing multi-col unique in %#v", orders.Indexes)
	}
}

func TestMSSQLDriverDSN(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{
			"sqlserver://user:pass@localhost:1433?database=mydb",
			"sqlserver://user:pass@localhost:1433?database=mydb",
		},
		{
			"mssql://user:pass@localhost:1433?database=mydb",
			"sqlserver://user:pass@localhost:1433?database=mydb",
		},
		{
			"MSSQL://sa@127.0.0.1?database=app",
			"sqlserver://sa@127.0.0.1?database=app",
		},
	}
	for _, tc := range cases {
		got, err := mssqlDriverDSN(tc.in)
		if err != nil {
			t.Fatalf("mssqlDriverDSN(%q): %v", tc.in, err)
		}
		if got != tc.want {
			t.Fatalf("mssqlDriverDSN(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestMSSQLFormatType(t *testing.T) {
	cases := []struct {
		name                string
		maxLen, prec, scale int
		want                string
	}{
		{"varchar", 50, 0, 0, "varchar(50)"},
		{"nvarchar", 100, 0, 0, "nvarchar(50)"},
		{"varchar", -1, 0, 0, "varchar(max)"},
		{"decimal", 0, 10, 2, "decimal(10,2)"},
		{"int", 4, 10, 0, "int"},
		{"datetime2", 0, 0, 7, "datetime2(7)"},
	}
	for _, tc := range cases {
		got := mssqlFormatType(tc.name, tc.maxLen, tc.prec, tc.scale)
		if got != tc.want {
			t.Fatalf("mssqlFormatType(%q,%d,%d,%d) = %q, want %q",
				tc.name, tc.maxLen, tc.prec, tc.scale, got, tc.want)
		}
	}
}

func TestMSSQLIntrospectIntegration(t *testing.T) {
	dsn := os.Getenv("ERDLENS_MSSQL_DSN")
	if dsn == "" {
		t.Skip("set ERDLENS_MSSQL_DSN to run MSSQL integration test")
	}
	ctx := context.Background()
	i, err := Open(ctx, dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = i.Close() }()

	m := i.(*MSSQL)
	_, err = m.db.ExecContext(ctx, `
IF OBJECT_ID('dbo.erdlens_orders', 'U') IS NOT NULL DROP TABLE dbo.erdlens_orders;
IF OBJECT_ID('dbo.erdlens_users', 'U') IS NOT NULL DROP TABLE dbo.erdlens_users;
CREATE TABLE dbo.erdlens_users (
  id INT NOT NULL PRIMARY KEY,
  email NVARCHAR(255) NOT NULL UNIQUE
);
CREATE TABLE dbo.erdlens_orders (
  id INT NOT NULL PRIMARY KEY,
  user_id INT NOT NULL,
  CONSTRAINT fk_erdlens_orders_user FOREIGN KEY (user_id) REFERENCES dbo.erdlens_users(id) ON DELETE CASCADE
);
`)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	t.Cleanup(func() {
		_, _ = m.db.ExecContext(ctx, `IF OBJECT_ID('dbo.erdlens_orders', 'U') IS NOT NULL DROP TABLE dbo.erdlens_orders`)
		_, _ = m.db.ExecContext(ctx, `IF OBJECT_ID('dbo.erdlens_users', 'U') IS NOT NULL DROP TABLE dbo.erdlens_users`)
	})

	s, err := i.Introspect(ctx, Options{Include: []string{"erdlens_*"}})
	if err != nil {
		t.Fatalf("introspect: %v", err)
	}
	if s.Dialect != "mssql" {
		t.Fatalf("dialect = %q", s.Dialect)
	}
	if len(s.Tables) < 2 {
		t.Fatalf("expected >=2 tables, got %d", len(s.Tables))
	}
	var foundUsers, foundOrders bool
	for _, tb := range s.Tables {
		if tb.Name == "erdlens_users" {
			foundUsers = true
			if tb.Schema != "dbo" {
				t.Fatalf("users schema = %q, want dbo", tb.Schema)
			}
			emailUnique := false
			for _, c := range tb.Columns {
				if c.Name == "email" && c.Unique {
					emailUnique = true
				}
			}
			if !emailUnique {
				t.Fatal("expected email Column.Unique")
			}
		}
		if tb.Name == "erdlens_orders" {
			foundOrders = true
			if len(tb.ForeignKeys) == 0 {
				t.Fatal("expected FK on erdlens_orders")
			}
			fk := tb.ForeignKeys[0]
			if fk.OnDelete != "cascade" {
				t.Fatalf("FK OnDelete = %q, want cascade", fk.OnDelete)
			}
		}
	}
	if !foundUsers || !foundOrders {
		t.Fatalf("missing seeded tables in %#v", s.Tables)
	}
}

func TestMySQLIntrospectIntegration(t *testing.T) {
	dsn := os.Getenv("ERDLENS_MYSQL_DSN")
	if dsn == "" {
		t.Skip("set ERDLENS_MYSQL_DSN to run MySQL integration test")
	}
	ctx := context.Background()
	i, err := Open(ctx, dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = i.Close() }()

	m := i.(*MySQL)
	_, err = m.db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS erdlens_users (
  id INT PRIMARY KEY,
  email VARCHAR(255) NOT NULL UNIQUE
) COMMENT='test users';
CREATE TABLE IF NOT EXISTS erdlens_orders (
  id INT PRIMARY KEY,
  user_id INT NOT NULL,
  CONSTRAINT fk_erdlens_orders_user FOREIGN KEY (user_id) REFERENCES erdlens_users(id) ON DELETE CASCADE
);
`)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	t.Cleanup(func() {
		_, _ = m.db.ExecContext(ctx, `DROP TABLE IF EXISTS erdlens_orders`)
		_, _ = m.db.ExecContext(ctx, `DROP TABLE IF EXISTS erdlens_users`)
	})

	s, err := i.Introspect(ctx, Options{Include: []string{"erdlens_*"}})
	if err != nil {
		t.Fatalf("introspect: %v", err)
	}
	if s.Dialect != "mysql" {
		t.Fatalf("dialect = %q", s.Dialect)
	}
	if len(s.Tables) < 2 {
		t.Fatalf("expected >=2 tables, got %d", len(s.Tables))
	}
	var foundUsers, foundOrders bool
	for _, tb := range s.Tables {
		if tb.Name == "erdlens_users" {
			foundUsers = true
			if tb.Schema == "" {
				t.Fatal("mysql table schema should be set")
			}
			if tb.Comment == "" {
				t.Log("warning: table comment empty (engine may strip)")
			}
		}
		if tb.Name == "erdlens_orders" {
			foundOrders = true
			if len(tb.ForeignKeys) == 0 {
				t.Fatal("expected FK on erdlens_orders")
			}
		}
	}
	if !foundUsers || !foundOrders {
		t.Fatalf("missing seeded tables in %#v", s.Tables)
	}
}
