package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/erdlens/erdlens/internal/erdfile"
	"github.com/erdlens/erdlens/internal/introspect"
	"github.com/erdlens/erdlens/internal/schema"
)

// resolveViewPath returns the .erd file to serve and an optional cleanup func.
// When dsn is set, introspects the database and writes a temp file (or -o path).
func resolveViewPath(
	ctx context.Context,
	fileArg, dsn, output string,
	schemas, include, exclude []string,
	timeout time.Duration,
) (path string, cleanup func(), err error) {
	switch {
	case dsn != "" && fileArg != "":
		return "", nil, errors.New("pass either a .erd file or --dsn, not both")
	case dsn != "":
		return materializeFromDSN(ctx, dsn, output, schemas, include, exclude, timeout)
	case fileArg != "":
		return fileArg, func() {}, nil
	default:
		return "", nil, errors.New("either a .erd file or --dsn is required")
	}
}

func materializeFromDSN(
	ctx context.Context,
	dsn, output string,
	schemas, include, exclude []string,
	timeout time.Duration,
) (path string, cleanup func(), err error) {
	s, err := introspectDSN(ctx, dsn, introspect.Options{
		Schemas: schemas,
		Include: include,
		Exclude: exclude,
	}, timeout)
	if err != nil {
		return "", nil, err
	}

	if output != "" {
		if err := writeSchema(output, s); err != nil {
			return "", nil, err
		}
		fmt.Fprintf(os.Stderr, "→ wrote %s\n", output)
		return output, func() {}, nil
	}

	f, err := os.CreateTemp("", "erdlens-*.erd")
	if err != nil {
		return "", nil, fmt.Errorf("create temp file: %w", err)
	}
	name := f.Name()
	if err := erdfile.Write(f, s); err != nil {
		_ = f.Close()
		_ = os.Remove(name)
		return "", nil, fmt.Errorf("write %s: %w", name, err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(name)
		return "", nil, err
	}

	fmt.Fprintf(os.Stderr, "→ introspected to %s (temporary; pass -o to save)\n", name)
	return name, func() { _ = os.Remove(name) }, nil
}

func writeSchema(path string, s *schema.Schema) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	if err := erdfile.Write(f, s); err != nil {
		_ = f.Close()
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := f.Close(); err != nil {
		return err
	}
	return nil
}
