// Package main is the entry point for the erdlens CLI.
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// version is set at build time via -ldflags "-X main.version=..."
var version = "dev"

func main() {
	root := &cobra.Command{
		Use:           "erdlens",
		Short:         "A lens on your ERD",
		Long:          "erdlens — interactive, git-friendly ER diagrams for relational databases.",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	root.AddCommand(newGenerateCmd())
	root.AddCommand(newViewCmd())
	// TODO(phase 3): connect command
	// TODO(v2): diff command

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
