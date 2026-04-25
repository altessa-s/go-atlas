// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package defaults

import (
	"github.com/altessa-s/go-atlas/tools/codegen/optgen/plugin"
)

// SerializerDefaultPlugin provides default values for serializer.Serializer types.
// It generates: &serializer.JSON{}
type SerializerDefaultPlugin struct{}

func (p *SerializerDefaultPlugin) Meta() plugin.Meta {
	return plugin.Meta{
		Kind:     plugin.KindTypeDefault,
		Priority: 0,
	}
}

func (p *SerializerDefaultPlugin) CanProvideDefault(typeStr string) bool {
	return typeStr == "serializer.Serializer"
}

func (p *SerializerDefaultPlugin) GetDefault(typeStr string) string {
	return "&serializer.JSON{}"
}

func init() {
	plugin.Register(&SerializerDefaultPlugin{})
}
