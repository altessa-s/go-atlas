// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package commands wires the cobra root command for goconfig
// and registers all subcommands (currently [convert]).
//
// [New] returns the root [cobra.Command] for embedding in larger
// CLI trees. [Run] is a convenience that sets os.Args and executes
// the command in one call, suitable for main() or tests:
//
//	if err := commands.Run(os.Args[1:]); err != nil {
//	    os.Exit(1)
//	}
package commands
