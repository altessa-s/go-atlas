// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// optgen generates functional option functions from struct field tags.
//
// Usage:
//
//	//go:generate go run github.com/altessa-s/go-atlas/cmd/optgen generate --type=options
//
// The generator looks for fields with `opt` tags in the specified struct type
// and generates corresponding WithXxx functions.
//
// # Tag Format
//
// The opt tag format is:
//
//	`opt:"Name"` or `opt:"-"`
//
// Use additional tags for configuration:
//   - optgen:"default=...,append,notnil,manual,..."
//   - optval:"trimspaces,lower,dedup,..."
//   - optcheck:"required,minlen=...,oneof=[...],..."
//
// # Examples
//
//	type options struct {
//	    logger     *slog.Logger  `opt:"Logger"`                                  // generates WithLogger
//	    timeout    time.Duration `opt:"Timeout" optgen:"default=30*time.Second"` // generates WithTimeout
//	    labels     []string      `opt:"Labels" optval:"trimspaces,lower,dedup"`  // value modifiers
//	    custom     string        `opt:"-"`                                       // skipped - implement manually
//	}
package main

import (
	"os"

	"github.com/altessa-s/go-atlas/tools/codegen/optgen/commands"
)

func main() {
	if err := commands.Run(os.Args[1:]); err != nil {
		os.Exit(1)
	}
}
