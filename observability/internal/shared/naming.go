// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package shared

import (
	"strings"

	corestrings "github.com/altessa-s/go-atlas/core/text/strings"
)

// Common separators for metric and tracer names.
const (
	// MetricSeparator is used for metric names: {serviceName}_{subsystem}_{name}
	MetricSeparator = '_'

	// TracerSeparator is used for tracer names: {scope}/{name}
	TracerSeparator = '/'
)

// BuildName constructs a full name from parts using the given separator.
// Empty parts are skipped.
//
// Example:
//
//	BuildName([]string{"myapp", "http", "requests"}, '_') // "myapp_http_requests"
//	BuildName([]string{"myapp", "", "requests"}, '_')    // "myapp_requests"
func BuildName(parts []string, separator byte) string {
	// Count non-empty parts for fast path
	nonEmpty := 0
	for _, part := range parts {
		if part != "" {
			nonEmpty++
		}
	}

	// Fast path: no parts
	if nonEmpty == 0 {
		return ""
	}

	// Fast path: single part
	if nonEmpty == 1 {
		for _, part := range parts {
			if part != "" {
				return part
			}
		}
	}

	// Use pooled StringBuilder for efficient concatenation
	return corestrings.BuildString(func(b *strings.Builder) {
		first := true
		for _, part := range parts {
			if part == "" {
				continue
			}
			if !first {
				b.WriteByte(separator)
			}
			b.WriteString(part)
			first = false
		}
	})
}

// BuildMetricName constructs a metric name using underscore separator.
// Format: {serviceName}_{subsystem}_{name}
func BuildMetricName(serviceName, subsystem, name string) string {
	return BuildName([]string{serviceName, subsystem, name}, MetricSeparator)
}

// BuildTracerName constructs a tracer name using slash separator.
// Format: {scope}/{name}
func BuildTracerName(scope, name string) string {
	return BuildName([]string{scope, name}, TracerSeparator)
}

// JoinScope joins two scope names with the appropriate separator.
// Used for nested scopes in both metrics and tracing.
func JoinScope(parent, child string, separator byte) string {
	if parent == "" {
		return child
	}
	if child == "" {
		return parent
	}
	return corestrings.BuildString(func(b *strings.Builder) {
		b.WriteString(parent)
		b.WriteByte(separator)
		b.WriteString(child)
	})
}

// JoinMetricScope joins metric scopes with underscore.
func JoinMetricScope(parent, child string) string {
	return JoinScope(parent, child, MetricSeparator)
}

// JoinTracerScope joins tracer scopes with slash.
func JoinTracerScope(parent, child string) string {
	return JoinScope(parent, child, TracerSeparator)
}
