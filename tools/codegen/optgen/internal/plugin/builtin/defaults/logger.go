// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package defaults

import (
	"github.com/altessa-s/go-atlas/tools/codegen/optgen/plugin"
)

// LoggerDefaultPlugin provides default values for *slog.Logger types.
// It generates: slog.New(slog.DiscardHandler)
type LoggerDefaultPlugin struct {
	typeDefaultBase
}

func (p *LoggerDefaultPlugin) CanProvideDefault(typeStr string) bool {
	return typeStr == "*slog.Logger"
}

func (p *LoggerDefaultPlugin) GetDefault(typeStr string) string {
	return "slog.New(slog.DiscardHandler)"
}

func init() {
	plugin.Register(&LoggerDefaultPlugin{})
}
