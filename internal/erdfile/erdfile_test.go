package erdfile

import (
	"bytes"
	"strings"
	"testing"

	"github.com/erdlens/erdlens/internal/schema"
)

func sampleSchema() *schema.Schema {
	return &schema.Schema{
		Dialect: "postgres",
		Tables: []schema.Table{
			{
				Name:   "users",
				Schema: "public",
				Columns: []schema.Column{
					{Name: "id", Type: "uuid", Nullable: false},
					{Name: "email", Type: "text", Nullable: false, Unique: true},
					{Name: "created_at", Type: "timestamptz", Nullable: false, Default: "now()"},
				},
				PrimaryKey: []string{"id"},
				Indexes: []schema.Index{
					{Name: "idx_users_email_lower", Columns: []string{"email"}},
				},
			},
			{
				Name:   "orders",
				Schema: "public",
				Columns: []schema.Column{
					{Name: "id", Type: "uuid", Nullable: false},
					{Name: "user_id", Type: "uuid", Nullable: false},
					{Name: "total_cents", Type: "integer", Nullable: false, Default: "0"},
				},
				PrimaryKey: []string{"id"},
				ForeignKeys: []schema.ForeignKey{
					{
						Name:       "fk_orders_user",
						Columns:    []string{"user_id"},
						RefTable:   "users",
						RefColumns: []string{"id"},
						OnDelete:   "cascade",
					},
				},
			},
		},
	}
}

func TestWriteDeterministic(t *testing.T) {
	s := sampleSchema()
	var a, b bytes.Buffer
	if err := Write(&a, s); err != nil {
		t.Fatalf("first write: %v", err)
	}
	if err := Write(&b, s); err != nil {
		t.Fatalf("second write: %v", err)
	}
	if a.String() != b.String() {
		t.Fatalf("write not deterministic:\n--- a ---\n%s\n--- b ---\n%s", a.String(), b.String())
	}
}

func TestWriteSortsTables(t *testing.T) {
	s := sampleSchema() // users, then orders
	var buf bytes.Buffer
	if err := Write(&buf, s); err != nil {
		t.Fatalf("write: %v", err)
	}
	out := buf.String()
	iOrders := strings.Index(out, `table "orders"`)
	iUsers := strings.Index(out, `table "users"`)
	if iOrders < 0 || iUsers < 0 {
		t.Fatalf("expected both tables in output, got:\n%s", out)
	}
	if iOrders > iUsers {
		t.Fatalf("expected orders before users (alphabetical), got:\n%s", out)
	}
}

const golden = `meta {
  dialect = "postgres"
}

table "orders" {

  column "id" {
    type = "uuid"
    null = false
  }

  column "user_id" {
    type = "uuid"
    null = false
  }

  column "total_cents" {
    type = "integer"
    null = false
    default = "0"
  }

  primary_key {
    columns = ["id"]
  }

  foreign_key "fk_orders_user" {
    columns     = ["user_id"]
    ref_table   = "users"
    ref_columns = ["id"]
    on_delete   = "cascade"
  }
}

table "users" {

  column "id" {
    type = "uuid"
    null = false
  }

  column "email" {
    type = "text"
    null = false
    unique = true
  }

  column "created_at" {
    type = "timestamptz"
    null = false
    default = "now()"
  }

  primary_key {
    columns = ["id"]
  }

  index "idx_users_email_lower" {
    columns = ["email"]
  }
}
`

func TestWriteGolden(t *testing.T) {
	var buf bytes.Buffer
	if err := Write(&buf, sampleSchema()); err != nil {
		t.Fatalf("write: %v", err)
	}
	got := buf.String()
	if got != golden {
		t.Fatalf("golden mismatch\n--- got ---\n%s\n--- want ---\n%s", got, golden)
	}
}

func TestRoundTrip(t *testing.T) {
	original := sampleSchema()
	var buf1 bytes.Buffer
	if err := Write(&buf1, original); err != nil {
		t.Fatalf("first write: %v", err)
	}
	first := buf1.String()

	parsed, err := Parse(strings.NewReader(first))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	var buf2 bytes.Buffer
	if err := Write(&buf2, parsed); err != nil {
		t.Fatalf("second write: %v", err)
	}
	if buf2.String() != first {
		t.Fatalf("round-trip differs:\n--- first ---\n%s\n--- second ---\n%s", first, buf2.String())
	}
}

func TestParseNotImplemented(t *testing.T) {
	// Parse is implemented as of Phase 2; ensure it accepts an empty document.
	s, err := Parse(strings.NewReader(""))
	if err != nil {
		t.Fatalf("empty parse: %v", err)
	}
	if s == nil || len(s.Tables) != 0 {
		t.Fatalf("expected empty schema, got %+v", s)
	}
}

func TestWriteViews(t *testing.T) {
	s := &schema.Schema{
		Dialect: "postgres",
		Views: []schema.View{
			{Name: "billing", Include: []string{"invoices*", "payments"}},
			{Name: "auth", Include: []string{"users*", "sessions*"}, Exclude: []string{"users_audit"}},
		},
	}
	var buf bytes.Buffer
	if err := Write(&buf, s); err != nil {
		t.Fatalf("write: %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, `view "auth"`) || !strings.Contains(got, `view "billing"`) {
		t.Fatalf("missing view blocks:\n%s", got)
	}
	iAuth := strings.Index(got, `view "auth"`)
	iBilling := strings.Index(got, `view "billing"`)
	if iAuth > iBilling {
		t.Fatal("views not sorted alphabetically")
	}
	if !strings.Contains(got, `exclude = ["users_audit"]`) {
		t.Fatalf("exclude clause missing:\n%s", got)
	}
}

func TestViewRoundTrip(t *testing.T) {
	original := &schema.Schema{
		Dialect: "postgres",
		Views: []schema.View{
			{Name: "auth", Include: []string{"users*"}, Exclude: []string{"users_audit"}},
			{Name: "core"}, // empty include = all
		},
		Tables: sampleSchema().Tables,
	}
	var buf1 bytes.Buffer
	if err := Write(&buf1, original); err != nil {
		t.Fatal(err)
	}
	parsed, err := Parse(strings.NewReader(buf1.String()))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var buf2 bytes.Buffer
	if err := Write(&buf2, parsed); err != nil {
		t.Fatal(err)
	}
	if buf1.String() != buf2.String() {
		t.Fatalf("view round-trip differs:\n--- first ---\n%s\n--- second ---\n%s", buf1.String(), buf2.String())
	}
}
