// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package prometheus

import (
	"strings"

	"github.com/altessa-s/go-atlas/core/collections/slices"
)

// BuildMetricName builds a metric name with optional namespace and subsystem prefixes.
// Parts are joined with underscores. Empty parts are skipped.
//
// Example:
//
//	BuildMetricName("myapp", "http", "requests_total") -> "myapp_http_requests_total"
//	BuildMetricName("", "grpc", "requests_total") -> "grpc_requests_total"
//	BuildMetricName("", "", "requests_total") -> "requests_total"
func BuildMetricName(namespace, subsystem, suffix string) string {
	parts := make([]string, 0, DefaultNamePartsCapacity)

	parts = slices.AppendIf(parts, namespace != "", namespace)
	parts = slices.AppendIf(parts, subsystem != "", subsystem)
	parts = slices.AppendIf(parts, suffix != "", suffix)

	if len(parts) == 0 {
		return ""
	}

	return strings.Join(parts, "_")
}

// BuildMetricNameWithDefault works like [BuildMetricName] but prepends
// defaultPrefix to suffix when both namespace and subsystem are empty.
// This ensures that metrics always carry a transport-identifying prefix
// even when the user omits namespace configuration.
//
// Example:
//
//	BuildMetricNameWithDefault("myapp", "http", "requests_total", "http_")
//	  // => "myapp_http_requests_total"
//	BuildMetricNameWithDefault("", "", "requests_total", "http_")
//	  // => "http_requests_total"
func BuildMetricNameWithDefault(namespace, subsystem, suffix, defaultPrefix string) string {
	name := BuildMetricName(namespace, subsystem, suffix)
	if name == "" && defaultPrefix != "" {
		return defaultPrefix + suffix
	}
	if name == suffix && defaultPrefix != "" {
		return defaultPrefix + suffix
	}
	return name
}
