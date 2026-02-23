// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package defaults

import (
	"strings"

	"github.com/altessa-s/go-atlas/tools/codegen/optgen/plugin"
)

// MapDefaultPlugin provides default values for map types.
// It generates: make(map[K]V)
type MapDefaultPlugin struct{}

func (p *MapDefaultPlugin) Meta() plugin.Meta {
	return plugin.Meta{
		Kind:     plugin.KindTypeDefault,
		Priority: 0,
	}
}

func (p *MapDefaultPlugin) CanProvideDefault(typeStr string) bool {
	return strings.HasPrefix(typeStr, "map[")
}

func (p *MapDefaultPlugin) GetDefault(typeStr string) string {
	return "make(" + typeStr + ")"
}

func init() {
	plugin.Register(&MapDefaultPlugin{})
}
