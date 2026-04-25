// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package commands

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/altessa-s/go-atlas/tools/codegen/optgen/plugin"

	_ "github.com/altessa-s/go-atlas/tools/codegen/optgen/internal/generator" // init side-effects register builtins
)

// NewListPlugins returns a cobra command that prints all registered plugins,
// type default providers, modifier plugins, and optcheck keys. External .so
// plugins can be loaded before listing via --plugin. The builtin plugins are
// registered through an init-time import of the generator package.
func NewListPlugins() *cobra.Command {
	var showAll bool
	var externalPlugins []string
	cmd := &cobra.Command{
		Use:   "list-plugins",
		Short: "List known plugin and provider type names",
		Long: strings.TrimSpace(`
List registered field plugins and type default providers by type name.
These names can be used with --disable-plugins.
`),
		RunE: func(cmd *cobra.Command, _ []string) error {
			plugin.SetWarningSink(func(format string, args ...any) {
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "optgen: WARN: "+format+"\n", args...)
			})

			if err := loadExternalPlugins(externalPlugins); err != nil {
				return err
			}

			fieldInfos := plugin.FieldPlugins()
			prov := plugin.KnownTypeDefaultPluginNames()

			if len(fieldInfos) == 0 && len(prov) == 0 && !showAll {
				// In normal execution, builtins are registered via generator import side-effects.
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "No plugins/providers registered.")
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "If you run as a library/test, ensure builtin plugins are imported.")
				return nil
			}

			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Field plugins:")
			for _, p := range fieldInfos {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  - %s (priority=%d)\n", p.Name, p.Priority)
			}

			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Type default providers:")
			for _, n := range prov {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  - %s\n", n)
			}

			modifiers := plugin.ModifierPlugins()
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Modifier plugins:")
			for _, m := range modifiers {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  - %s (key=%q, phase=%s)\n", m.Name, m.Key, m.Phase)
			}

			checks := plugin.KnownCheckKeys()
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Optcheck keys:")
			for _, k := range checks {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  - %s\n", k)
			}

			return nil
		},
	}

	cmd.Flags().BoolVar(&showAll, "show-empty", false, "Show headers even when no plugins/providers are registered")
	cmd.Flags().StringArrayVar(&externalPlugins, "plugin", nil, "Load external plugins (.so) before listing (darwin/linux only)")

	return cmd
}
