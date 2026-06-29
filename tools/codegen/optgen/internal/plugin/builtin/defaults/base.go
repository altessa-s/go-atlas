// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package defaults

import (
	"github.com/altessa-s/go-atlas/tools/codegen/optgen/plugin"
)

// typeDefaultBase supplies the Meta shared by every type-default plugin in this
// package: Kind=typedefault, Priority=0. Embed it and implement
// CanProvideDefault/GetDefault.
type typeDefaultBase struct{}

func (typeDefaultBase) Meta() plugin.Meta {
	return plugin.Meta{
		Kind:     plugin.KindTypeDefault,
		Priority: 0,
	}
}
