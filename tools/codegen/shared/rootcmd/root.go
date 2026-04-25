// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package rootcmd

import (
	"fmt"
	"io"
	"os"
	"runtime/debug"

	"github.com/MakeNowJust/heredoc"
	"github.com/spf13/cobra"

	"github.com/altessa-s/go-atlas/core/runtime/appinfo"
)

// Command wraps a [cobra.Command] with shared pre-run and post-run hooks
// including panic recovery.
type Command struct {
	*cobra.Command
}

// Config holds the declarative specification for a root command.
// At minimum, Use and Short must be set. Subcommands are added in
// the order provided. Out and Err default to [os.Stdout] and [os.Stderr]
// when nil.
type Config struct {
	Use         string
	Short       string
	Long        string
	Example     string
	Subcommands []*cobra.Command
	Out         io.Writer
	Err         io.Writer
}

// New builds and returns a root [cobra.Command] from cfg. It panics if cfg
// is nil. The returned command has SilenceUsage enabled, a version template
// sourced from [appinfo.Version], and a PersistentPreRunE that recovers
// panics from any subcommand's RunE.
func New(cfg *Config) *cobra.Command {
	if cfg == nil {
		panic("rootcmd: cfg cannot be nil")
	}

	root := &Command{
		Command: &cobra.Command{
			Use:          cfg.Use,
			Short:        cfg.Short,
			Long:         cfg.Long,
			Example:      cfg.Example,
			SilenceUsage: true,
			Version:      appinfo.Version,
		},
	}

	root.configure(cfg.Subcommands)
	if cfg.Out != nil {
		root.SetOut(cfg.Out)
	}
	if cfg.Err != nil {
		root.SetErr(cfg.Err)
	}

	return root.Command
}

// configure sets up the root command with all its subcommands and configurations.
func (c *Command) configure(subcommands []*cobra.Command) {
	// Set standard streams
	// Defaults are overridden by Run when Config provides custom writers.
	c.SetErr(os.Stderr)
	c.SetOut(os.Stdout)

	// Configure command suggestions
	c.SuggestionsMinimumDistance = 1

	// Add version template
	c.SetVersionTemplate(heredoc.Doc(`
		{{with .Name}}{{printf "%s " .}}{{end}}{{printf "version %s" .Version}}
	`))

	// Set pre-run hook with panic recovery wrapper
	c.PersistentPreRunE = c.preRunWithRecovery

	// Set post-run hook for cleanup
	c.PersistentPostRun = c.postRun

	// Add subcommands
	if len(subcommands) > 0 {
		c.AddCommand(subcommands...)
	}
}

// preRunWithRecovery wraps preRun with panic recovery mechanism.
func (c *Command) preRunWithRecovery(cmd *cobra.Command, args []string) (err error) {
	defer func() {
		if r := recover(); r != nil {
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Panic recovered: %v\n", r)
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Stack trace:\n%s\n", debug.Stack())
			err = fmt.Errorf("panic recovered: %v", r)
		}
	}()
	return c.preRun(cmd, args)
}

// preRun performs common initialization before any command execution.
func (c *Command) preRun(_ *cobra.Command, _ []string) error { return nil }

// postRun performs cleanup after command execution.
func (c *Command) postRun(_ *cobra.Command, _ []string) {}

// Run creates a root command from cfg, sets its args, and executes it.
// A top-level deferred recover wraps the entire Execute call, so panics
// that escape PersistentPreRunE (e.g., in a subcommand's RunE) are still
// caught, logged to stderr, and returned as an error.
func Run(cfg *Config, args []string) (err error) {
	cmd := New(cfg)
	cmd.SetArgs(args)

	defer func() {
		if r := recover(); r != nil {
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Panic recovered: %v\n", r)
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Stack trace:\n%s\n", debug.Stack())
			err = fmt.Errorf("panic recovered: %v", r)
		}
	}()

	return cmd.Execute()
}
