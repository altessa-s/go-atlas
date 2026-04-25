// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package generator

import "github.com/altessa-s/go-atlas/tools/codegen/optgen/plugin"

// Generator generates option functions from parsed field information.
type Generator struct {
}

// New creates a new Generator.
func New() *Generator {
	return &Generator{}
}

// DisablePlugins disables specified plugins by name.
// This is useful when you want to skip certain plugins during code generation.
//
// Example:
//
//	g.DisablePlugins("UtilityPostProcessor")
func (g *Generator) DisablePlugins(names ...string) {
	plugin.DisablePlugins(names...)
}
