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
	)

	cmd := &cobra.Command{
		Use:   "view <file.erd>",
		Short: "Open the interactive viewer for a local .erd file",
		Long: `Load a local .erd file, spin up a localhost HTTP server, and open the
embedded Svelte viewer in a browser. When you rearrange tables in the UI, the
new positions are written back to the source file via POST /api/layout.

The server binds to 127.0.0.1 only. No external services are contacted.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := args[0]
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
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := erdfile.Write(f, s); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
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
