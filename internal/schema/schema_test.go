package schema

import "testing"

func TestTableID(t *testing.T) {
	cases := []struct {
		t    Table
		want string
	}{
		{Table{Name: "users"}, "users"},
		{Table{Name: "users", Schema: "public"}, "users"},
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
		{ForeignKey{RefSchema: "auth", RefTable: "users"}, "auth.users"},
	}
	for _, tc := range cases {
		if got := RefTableID(tc.fk); got != tc.want {
			t.Fatalf("RefTableID(%+v) = %q, want %q", tc.fk, got, tc.want)
		}
	}
}
