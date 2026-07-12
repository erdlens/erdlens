package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	erdlens "github.com/erdlens/erdlens"
	"github.com/erdlens/erdlens/internal/erdfile"
	"github.com/erdlens/erdlens/internal/schema"
	"github.com/erdlens/erdlens/internal/server"
)

func newViewCmd() *cobra.Command {
	var (
		addr      string
		noBrowser bool
		dsn       string
		output    string
		schemas   []string
		include   []string
		exclude   []string
		timeout   time.Duration
	)

	cmd := &cobra.Command{
		Use:   "view [file.erd]",
		Short: "Open the interactive viewer for a .erd file or live database",
		Long: `Load a schema and open the embedded Svelte viewer in a browser.

Pass a local .erd file, or use --dsn to introspect a live database in one step
(written to a temp file under /tmp, or to -o if you want to keep it).

When you rearrange tables in the UI, positions are written back to the source
file via POST /api/layout.

The server binds to 127.0.0.1 only.`,
		Example: `  erdlens view schema.erd
  erdlens view --dsn 'postgres://user:pass@localhost:5432/mydb'
  erdlens view --dsn "$DATABASE_URL" -o schema.erd`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var fileArg string
			if len(args) > 0 {
				fileArg = args[0]
			}

			path, cleanup, err := resolveViewPath(
				cmd.Context(), fileArg, dsn, output,
				schemas, include, exclude, timeout,
			)
			if err != nil {
				return err
			}
			defer cleanup()

			sc, err := loadSchema(path)
			if err != nil {
				return err
			}

			assets, err := erdlens.Assets()
			if err != nil {
				return fmt.Errorf("load assets: %w", err)
			}

			onSave := func(s *schema.Schema) error {
				return saveSchema(path, s)
			}

			srv, err := server.Start(cmd.Context(), server.Config{
				Addr:   addr,
				Assets: assets,
				Schema: sc,
				OnSave: onSave,
			})
			if err != nil {
				return fmt.Errorf("start server: %w", err)
			}

			url := srv.Addr()
			fmt.Fprintf(os.Stderr, "erdlens viewer serving %s → %s\n", path, url)
			fmt.Fprintln(os.Stderr, "press Ctrl-C to stop")

			if !noBrowser {
				openBrowser(url)
			}

			sig := make(chan os.Signal, 1)
			signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
			<-sig

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			return srv.Shutdown(ctx)
		},
	}

	cmd.Flags().StringVar(&addr, "addr", "127.0.0.1:0", "Address to bind (host:port, 0 = pick a free port)")
	cmd.Flags().BoolVar(&noBrowser, "no-browser", false, "Don't automatically open a browser")
	cmd.Flags().StringVar(&dsn, "dsn", "", "Introspect a live database instead of reading a file")
	cmd.Flags().StringVarP(&output, "output", "o", "", "When using --dsn, write the .erd file here (default: temp file in /tmp)")
	cmd.Flags().StringSliceVar(&schemas, "schema", []string{"public"}, "Schemas to include when using --dsn (Postgres)")
	cmd.Flags().StringSliceVar(&include, "include", nil, "Glob patterns of table names to include when using --dsn")
	cmd.Flags().StringSliceVar(&exclude, "exclude", nil, "Glob patterns of table names to exclude when using --dsn")
	cmd.Flags().DurationVar(&timeout, "timeout", 30*time.Second, "Connection + introspection timeout when using --dsn")

	return cmd
}

func loadSchema(path string) (*schema.Schema, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()
	sc, err := erdfile.Parse(f)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return sc, nil
}

func saveSchema(path string, s *schema.Schema) error {
	return writeSchema(path, s)
}

func openBrowser(url string) {
	var c *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		c = exec.Command("open", url)
	case "linux":
		c = exec.Command("xdg-open", url)
	case "windows":
		c = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		return
	}
	_ = c.Start()
}
