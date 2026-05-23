// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package leveled

//go:generate go run github.com/altessa-s/go-atlas/cmd/optgen generate

import (
	"log/slog"
	"maps"

	slogx "github.com/altessa-s/go-atlas/observability/slog"
)

// DefaultLevel is the default log level applied to subsystems
// not present in the levels map.
const DefaultLevel = slog.LevelInfo

// DefaultSubsystemKey is the default attribute key used to identify subsystems.
const DefaultSubsystemKey = slogx.ModuleKey

// options configures per-subsystem level filtering.
type options struct {
	// defaultLevel is the fallback level for subsystems not in the levels map.
	defaultLevel slog.Level `optgen:"default=DefaultLevel"`

	// subsystemLevels maps subsystem names to their minimum log levels.
	subsystemLevels map[string]slog.Level `optgen:"manual"`

	// subsystemKey is the attribute key used to identify subsystems.
	subsystemKey string `optgen:"default=DefaultSubsystemKey"`
}

// WithSubsystemLevels sets the per-subsystem level overrides.
// The map is copied; subsequent mutations of m have no effect.
func WithSubsystemLevels(m map[string]slog.Level) Option {
	return func(o *options) {
		o.subsystemLevels = make(map[string]slog.Level, len(m))
		maps.Copy(o.subsystemLevels, m)
	}
}
