// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"log/slog"
)

// UseLogger sets the logger for the builder and all created components.
func (b *TracerBuilder) UseLogger(v *slog.Logger) *TracerBuilder {
	b.SetLogger(v)
	return b
}

// UseDefaultLogger sets the logger to [slog.Default].
func (b *TracerBuilder) UseDefaultLogger() *TracerBuilder {
	return b.UseLogger(slog.Default())
}
