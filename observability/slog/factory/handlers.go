// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"io"
	"log/slog"
	"sync"

	"github.com/altessa-s/go-atlas/config"
)

// HandlerFactory creates a [slog.Handler] for a specific log format.
// Register custom factories via [RegisterHandler].
type HandlerFactory func(w io.Writer, cfg *config.Logger, opts *slog.HandlerOptions) slog.Handler

var (
	// customHandlers is a registry of custom log handler factories.
	customHandlers   = make(map[config.LogFormat]HandlerFactory)
	customHandlersMu sync.RWMutex
)

// RegisterHandler registers a custom [HandlerFactory] for the given log format.
// Safe for concurrent use.
func RegisterHandler(format config.LogFormat, factory HandlerFactory) {
	customHandlersMu.Lock()
	defer customHandlersMu.Unlock()
	customHandlers[format] = factory
}
