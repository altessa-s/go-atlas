// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config

// HttpInterGeoAclConfig defines the configuration for the HTTP geographic access control middleware.
type HttpInterGeoAclConfig struct {
	HttpInterAclConfig[GeoAclRuleConfig] `yaml:",inline"`
}

// Validate performs validation of the HTTP geographic access control middleware configuration.
func (c *HttpInterGeoAclConfig) Validate() error {
	return c.ValidateWith(validateGeoAclRule)
}

// DefaultHttpInterGeoAclConfig returns a configuration for GeoACL middleware with default values.
func DefaultHttpInterGeoAclConfig() HttpInterGeoAclConfig {
	return HttpInterGeoAclConfig{
		HttpInterAclConfig: DefaultHttpInterAclConfig[GeoAclRuleConfig](),
	}
}
