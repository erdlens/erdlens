package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/erdlens/erdlens/internal/erdfile"
	"github.com/erdlens/erdlens/internal/introspect"
)

func newGenerateCmd() *cobra.Command {
	var (
		dsn     string
		output  string
		schemas []string
		include []string
		exclude []string
		timeout time.Duration
	)

	cmd := &cobra.Command{
		Use:   "generate",
		Short: "Introspect a live DB and write a versionable .erd file",
		Long: `Connect to a live database, introspect its schema, and emit a
human-readable, git-versionable .erd file (HCL format).

Running 'generate' twice against the same database must produce byte-identical
output — this is the promise that keeps diffs clean.`,
		Example: `  erdlens generate --dsn 'postgres://user:pass@host/db' -o schema.erd
  erdlens generate --dsn "$DATABASE_URL" --schema public --exclude 'audit_*' -o schema.erd`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if dsn == "" {
				return errors.New("--dsn is required")
			}

			ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
			defer cancel()

			ins, err := introspect.Open(ctx, dsn)
			if err != nil {
				return fmt.Errorf("connect: %w", err)
			}
			defer func() { _ = ins.Close() }()

			s, err := ins.Introspect(ctx, introspect.Options{
				Schemas: schemas,
				Include: include,
				Exclude: exclude,
			})
			if err != nil {
				return fmt.Errorf("introspect: %w", err)
			}

			out, closeOut, err := openOutput(output)
			if err != nil {
				return err
			}
			defer closeOut()

			if err := erdfile.Write(out, s); err != nil {
				return fmt.Errorf("write: %w", err)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&dsn, "dsn", "", "Database DSN (e.g. postgres://user:pass@host/db)")
	cmd.Flags().StringVarP(&output, "output", "o", "", "Output .erd file path (default: stdout)")
	cmd.Flags().StringSliceVar(&schemas, "schema", []string{"public"}, "Schemas to include (Postgres)")
	cmd.Flags().StringSliceVar(&include, "include", nil, "Glob patterns of table names to include")
	cmd.Flags().StringSliceVar(&exclude, "exclude", nil, "Glob patterns of table names to exclude")
	cmd.Flags().DurationVar(&timeout, "timeout", 30*time.Second, "Overall connection + introspection timeout")

	return cmd
}

// openOutput returns a writer for the requested destination and a cleanup func.
// "" or "-" mean stdout.
func openOutput(path string) (io.Writer, func(), error) {
	if path == "" || path == "-" {
		return os.Stdout, func() {}, nil
	}
	f, err := os.Create(path)
	if err != nil {
		return nil, nil, fmt.Errorf("create %s: %w", path, err)
	}
	return f, func() { _ = f.Close() }, nil
}
