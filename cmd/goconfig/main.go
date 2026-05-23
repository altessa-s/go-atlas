// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// goconfig is a tool for converting configuration between formats.
//
// It supports conversion between Go structs, environment variables,
// and markdown documentation.
package main

import (
	"os"

	"github.com/altessa-s/go-atlas/tools/codegen/goconfig/commands"
)

func main() {
	if err := commands.Run(os.Args[1:]); err != nil {
		os.Exit(1)
	}
}
