// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"log/slog"

	"github.com/altessa-s/go-atlas/data/audit"
)

// UseLogger sets the logger for the builder and all created components.
func (b *AuditorBuilder) UseLogger(v *slog.Logger) *AuditorBuilder {
	b.SetLogger(v)
	return b
}

// UseDefaultLogger sets the logger to [slog.Default].
func (b *AuditorBuilder) UseDefaultLogger() *AuditorBuilder {
	return b.UseLogger(slog.Default())
}

// UseDispatcher sets the [audit.Dispatcher] that the Auditor will use for
// async event dispatch. The dispatcher must already be started.
func (b *AuditorBuilder) UseDispatcher(v audit.Dispatcher) *AuditorBuilder {
	b.dispatcher = v
	return b
}
