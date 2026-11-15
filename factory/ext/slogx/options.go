// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"strings"
)

// --- Configuration methods ---

// WithPrefixKey sets the attribute key for prefix values. Default [slogx.ModuleKey].
func (b *LoggerBuilder) WithPrefixKey(v string) *LoggerBuilder {
	if v = strings.TrimSpace(v); v != "" {
		b.prefixKey = v
	}
	return b
}

// WithEnableMasking enables the advanced masking handler wrapper, which
// applies a curated default sensitive-field set (`password`, `token`,
// `secret`, common patterns like `*api_key*`, ...) on top of any tags
// declared in `observabilityconfig.Logger.SensitiveTags`.
//
// The wrapper is also enabled automatically when `observabilityconfig.Logger.SensitiveTags`
// is non-empty, so this option is mainly useful for callers that want the
// default field set without listing tags themselves.
func (b *LoggerBuilder) WithEnableMasking() *LoggerBuilder {
	b.enableMasking = true
	return b
}

// WithAppName sets the application name to include in logs.
func (b *LoggerBuilder) WithAppName(v string) *LoggerBuilder {
	if v = strings.TrimSpace(v); v != "" {
		b.appName = v
	}
	return b
}

// WithAppVersion sets the application version to include in logs.
func (b *LoggerBuilder) WithAppVersion(v string) *LoggerBuilder {
	if v = strings.TrimSpace(v); v != "" {
		b.appVersion = v
	}
	return b
}

// WithServiceId sets the service ID to include in logs.
func (b *LoggerBuilder) WithServiceId(v string) *LoggerBuilder {
	if v = strings.TrimSpace(v); v != "" {
		b.serviceID = v
	}
	return b
}
