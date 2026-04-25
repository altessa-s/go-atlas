// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package commands

import (
	"github.com/MakeNowJust/heredoc"
	"github.com/spf13/cobra"

	"github.com/altessa-s/go-atlas/tools/codegen/shared/rootcmd"
)

// config returns the root command configuration.
func config() rootcmd.Config {
	return rootcmd.Config{
		Use:   "optgen",
		Short: "Generate functional option functions from struct tags",
		Long: heredoc.Doc(`
			optgen generates functional option functions from struct field tags.

			The generator looks for fields with 'opt' tags in the specified struct type
			and generates corresponding inline WithXxx functions.

			Tag Format:
			  opt:"Name"  - Generate option
			  opt:"-"     - Skip generation

			Examples:
			  type options struct {
			      logger     *slog.Logger     opt:"Logger"                     // generates WithLogger
			      timeout    time.Duration    opt:"Timeout" optgen:"default=30*time.Second" // generates WithTimeout
			      hosts      []string         opt:"Hosts" optgen:"append"      // slice append semantics
			      name       string           opt:"Name" optval:"trimspaces,lower" // value modifiers
			      custom     string           opt:"-"                          // skipped - implement manually
			  }
		`),
		Example: heredoc.Doc(`
			# Generate options for a struct type
			optgen generate --type options

			# Generate with custom output file
			optgen generate --type options --output my_options_gen.go

			# Preview changes before committing (dry-run mode)
			optgen generate --type options --dry-run

			# Show detailed generation progress (verbose mode)
			optgen generate --type options --verbose

			# Combine dry-run with verbose for complete visibility
			optgen generate --type options --dry-run --verbose

			# Show version information
			optgen version
		`),
		Subcommands: []*cobra.Command{
			NewGenerate(),
			NewListPlugins(),
		},
	}
}

// New creates a new root command.
func New() *cobra.Command {
	cfg := config()
	return rootcmd.New(&cfg)
}

// Run sets the arguments and executes the root command.
// This function is used for testing and when arguments need to be set explicitly.
func Run(args []string) error {
	cfg := config()
	return rootcmd.Run(&cfg, args)
}
