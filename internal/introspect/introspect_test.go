package introspect

import "testing"

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
	_, err := Open(nil, "mysql://foo/bar")
	if err == nil {
		t.Fatal("expected error for unsupported dialect")
	}
}
