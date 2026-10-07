// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package middlewareconfig

// EnableMixin provides a common Enabled field for configurations that can be toggled.
// Embed this struct with `yaml:",inline"` to add an Enabled field to your config.
//
// Note: Each config should still implement its own IsEnabled() method for nil-safety:
//
//	func (c *MyConfig) IsEnabled() bool {
//	    return c != nil && c.Enabled
//	}
//
// Example:
//
//	type MyInterceptorConfig struct {
//	    EnableMixin `yaml:",inline"`
//	    // ... other fields
//	}
type EnableMixin struct {
	// Enabled controls whether this feature is active.
	// When false, the feature is disabled and has no effect.
	Enabled bool `yaml:"enabled" default:"false"`
}
