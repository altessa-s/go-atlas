// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package prefixed

//go:generate go run github.com/altessa-s/go-atlas/cmd/optgen generate

import (
	"log/slog"
	"strings"

	"github.com/altessa-s/go-atlas/core/collections/slices"

	stdSlices "slices"
)

// DefaultPrefixesDelimiter separates multiple prefixes.
const DefaultPrefixesDelimiter = ":"

// Formatter transforms prefix values into a single formatted prefix value.
type Formatter func([]slog.Value, string) *slog.Value

// options configures how the prefixed handler processes prefixes.
type options struct {
	// prefixFormatter formats multiple prefixes into one. Nil uses DefaultFormatter.
	prefixFormatter Formatter `optgen:"default=DefaultFormatter" optval:"nonnil"`

	// prefix is the attribute key for prefix values.
	prefix string `optval:"nonempty"`

	// prefixesDelimiter joins multiple prefixes. Default ":".
	prefixesDelimiter string `optgen:"default=DefaultPrefixesDelimiter"`
}

// prefixFormatter joins prefix values with delimiter, filtering empty values.
func formatPrefixes(prefixes []slog.Value, delimiter string) string {
	if len(prefixes) == 0 {
		return ""
	}

	p := stdSlices.Collect(slices.Map(stdSlices.Collect(slices.Filter(prefixes, func(v slog.Value) bool {
		return v.Any() != nil && v.String() != ""
	})), func(v slog.Value) string { return v.String() }))

	if len(p) == 0 {
		return ""
	}

	return strings.Join(p, delimiter)
}

// DefaultFormatter formats prefixes with square brackets, e.g., "[api:server]".
func DefaultFormatter(prefixes []slog.Value, delimiter string) *slog.Value {
	val := slog.StringValue("[" + formatPrefixes(prefixes, delimiter) + "]")
	return &val
}

// JsonFormatter formats prefixes without brackets, e.g., "api:server".
func JsonFormatter(prefixes []slog.Value, delimiter string) *slog.Value {
	val := slog.StringValue(formatPrefixes(prefixes, delimiter))
	return &val
}
