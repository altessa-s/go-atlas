// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package colorized

//go:generate go run github.com/altessa-s/go-atlas/cmd/optgen generate

import (
	"log/slog"
	"time"
)

const (
	// DefaultTimeFormat is the default time format for log timestamps (RFC3339Nano).
	DefaultTimeFormat = time.RFC3339Nano
	// DefaultLogLevel is the default minimum log level (slog.LevelError).
	DefaultLogLevel = slog.LevelError
)

// options configures the behavior of the colorized handler.
type options struct {
	// addSource enables adding source location (file, line, function) to output.
	addSource bool

	// level is the minimum log level to process. Default is slog.LevelError.
	level slog.Leveler `optgen:"default=DefaultLogLevel" optval:"nil"`

	// replaceAttr transforms or filters attributes before logging.
	// Return empty slog.Attr{} to remove an attribute.
	replaceAttr func(groups []string, attr slog.Attr) slog.Attr `optgen:"manual"`

	// noColor disables ANSI color output. Default is false (colors enabled).
	noColor bool

	// timeFormat uses Go's time.Format pattern. Default is time.RFC3339Nano.
	timeFormat string `optgen:"default=DefaultTimeFormat"`

	// attributeColors maps attribute keys to color.Attribute slices.
	attributeColors map[string][]int

	// prefixAttributeKey defines an attribute to display as a prefix before the message.
	prefixAttributeKey string
}

// WithReplaceAttr sets a function to transform or filter attributes before output.
func WithReplaceAttr(replace func(groups []string, attr slog.Attr) slog.Attr) Option {
	return func(o *options) {
		o.replaceAttr = replace
	}
}
