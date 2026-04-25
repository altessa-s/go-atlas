// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package rootcmd provides a reusable foundation for cobra-based CLI tools
// in the go-atlas codegen family. It handles version display (sourced from
// [appinfo.Version]), command-suggestion distance, and panic recovery with
// full stack traces written to stderr.
//
// [New] returns a configured [cobra.Command] that callers can extend or
// execute directly. [Run] is a convenience wrapper that sets args, installs
// an additional top-level panic recovery, and calls Execute:
//
//	cfg := rootcmd.Config{
//	    Use:         "mytool",
//	    Short:       "A sample CLI tool",
//	    Subcommands: []*cobra.Command{generateCmd, convertCmd},
//	}
//	if err := rootcmd.Run(&cfg, os.Args[1:]); err != nil {
//	    os.Exit(1)
//	}
//
// Custom [io.Writer] values for stdout and stderr can be supplied via
// [Config.Out] and [Config.Err], which is useful for capturing output in tests.
package rootcmd
