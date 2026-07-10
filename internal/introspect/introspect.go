// Package introspect defines the driver-agnostic interface for reading
// a live database into the canonical schema IR.
//
// Each supported dialect provides an Introspector implementation
// (see postgres.go, mysql.go, ...). The concrete constructor is chosen
// by parsing the DSN scheme.
package introspect

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"path"
	"strings"

	"github.com/erdlens/erdlens/internal/schema"
)

// Introspector reads schema metadata from a live database.
type Introspector interface {
	// Introspect returns the canonical schema representation for the connected DB.
	// Callers can restrict scope with Options (schemas, include/exclude globs).
	Introspect(ctx context.Context, opts Options) (*schema.Schema, error)
	Close() error
}

// Options controls which objects are included in introspection.
type Options struct {
	Schemas []string // e.g. ["public"] for Postgres. Empty → driver default.
	Include []string // Glob patterns of table names to include. Empty → all.
	Exclude []string // Glob patterns of table names to exclude. Wins over Include.
}

// Match reports whether the given table name should be kept given the
// include/exclude globs. Exclude wins over include. Glob syntax is
// path.Match (supports *, ?, [class]).
func (o Options) Match(name string) bool {
	for _, p := range o.Exclude {
		if ok, _ := path.Match(p, name); ok {
			return false
		}
	}
	if len(o.Include) == 0 {
		return true
	}
	for _, p := range o.Include {
		if ok, _ := path.Match(p, name); ok {
			return true
		}
	}
	return false
}

// ErrUnsupportedDialect is returned when the DSN scheme is not recognized.
var ErrUnsupportedDialect = errors.New("unsupported dialect")

// Open selects a driver based on the DSN scheme and returns a connected Introspector.
func Open(ctx context.Context, dsn string) (Introspector, error) {
	u, err := url.Parse(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse dsn: %w", err)
	}
	switch strings.ToLower(u.Scheme) {
	case "postgres", "postgresql":
		return NewPostgres(ctx, dsn)
	default:
		return nil, fmt.Errorf("%w: %q (supported: postgres)", ErrUnsupportedDialect, u.Scheme)
	}
}
