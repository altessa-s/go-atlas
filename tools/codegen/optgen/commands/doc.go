// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package commands implements the cobra-based CLI for the optgen code generator.
//
// The package exposes three entry points:
//
//   - [New] returns a ready-to-execute [cobra.Command] tree (generate, list-plugins, version).
//   - [Run] is a convenience wrapper that sets os.Args and executes the root command.
//   - [NewGenerate] and [NewListPlugins] return individual subcommands.
//
// Configuration is loaded from .optgen.yaml (see [config.Config]) unless --no-config is set.
// CLI flags always take precedence over config-file values.
//
// External plugins (.so files built with buildmode=plugin) can be loaded via --plugin on
// darwin and linux. On other platforms the flag returns an error.
package commands
