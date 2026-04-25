// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package commands

import (
	"github.com/MakeNowJust/heredoc"
	"github.com/spf13/cobra"

	"github.com/altessa-s/go-atlas/tools/codegen/goconfig/commands/convert"
	"github.com/altessa-s/go-atlas/tools/codegen/shared/rootcmd"
)

// config returns the root command configuration.
func config() rootcmd.Config {
	return rootcmd.Config{
		Use:   "goconfig",
		Short: "Configuration file converter for go-tools config parser",
		Long: heredoc.Doc(`
			goconfig is a CLI utility for converting between different configuration formats.

			It supports conversion between:
			- YAML configuration files
			- .env files (environment variables)

			The converter understands the go-tools config parser structure:
			- Uses __ (double underscore) as section delimiter
			- Converts camelCase field names to SCREAMING_SNAKE_CASE
			- Properly handles nested structures and arrays
		`),
		Example: heredoc.Doc(`
			# Convert YAML to .env
			goconfig convert --from config.yaml --to .env

			# Convert .env to YAML
			goconfig convert --from .env --to config.yaml

			# Show version information
			goconfig version
		`),
		Subcommands: []*cobra.Command{
			convert.New(),
		},
	}
}

// New returns the goconfig root [cobra.Command] with the convert subcommand
// already registered. The returned command can be embedded in a larger CLI tree
// or executed directly via [cobra.Command.Execute].
func New() *cobra.Command {
	cfg := config()
	return rootcmd.New(&cfg)
}

// Run creates a fresh root command, sets args, and executes it.
// It returns any error produced by the command tree, including panics
// recovered by [rootcmd.Run].
func Run(args []string) error {
	cfg := config()
	return rootcmd.Run(&cfg, args)
}
