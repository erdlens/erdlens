package schema

import "testing"

func TestTableID(t *testing.T) {
	cases := []struct {
		t    Table
		want string
	}{
		{Table{Name: "users"}, "users"},
		{Table{Name: "users", Schema: "public"}, "users"},
		{Table{Name: "users", Schema: "dbo"}, "users"},
		{Table{Name: "users", Schema: "auth"}, "auth.users"},
	}
	for _, tc := range cases {
		if got := TableID(tc.t); got != tc.want {
			t.Fatalf("TableID(%+v) = %q, want %q", tc.t, got, tc.want)
		}
	}
}

func TestRefTableID(t *testing.T) {
	cases := []struct {
		fk   ForeignKey
		want string
	}{
		{ForeignKey{RefTable: "users"}, "users"},
		{ForeignKey{RefSchema: "public", RefTable: "users"}, "users"},
		{ForeignKey{RefSchema: "dbo", RefTable: "users"}, "users"},
		{ForeignKey{RefSchema: "auth", RefTable: "users"}, "auth.users"},
	}
	for _, tc := range cases {
		if got := RefTableID(tc.fk); got != tc.want {
			t.Fatalf("RefTableID(%+v) = %q, want %q", tc.fk, got, tc.want)
		}
	}
}

func TestSQLViewID(t *testing.T) {
	cases := []struct {
		v    SQLView
		want string
	}{
		{SQLView{Name: "v"}, "v"},
		{SQLView{Name: "v", Schema: "public"}, "v"},
		{SQLView{Name: "v", Schema: "dbo"}, "v"},
		{SQLView{Name: "v", Schema: "auth"}, "auth.v"},
	}
	for _, tc := range cases {
		if got := SQLViewID(tc.v); got != tc.want {
			t.Fatalf("SQLViewID(%+v) = %q, want %q", tc.v, got, tc.want)
		}
	}
}
