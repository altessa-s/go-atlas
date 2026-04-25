// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config

// HttpInterIpAclConfig defines the configuration for the HTTP IP access control middleware.
type HttpInterIpAclConfig struct {
	HttpInterAclConfig[IpAclRuleConfig] `yaml:",inline"`
}

// Validate performs validation of the HTTP IP access control middleware configuration.
func (c *HttpInterIpAclConfig) Validate() error {
	return c.ValidateWith(validateIpAclRule)
}

// DefaultHttpInterIpAclConfig returns a configuration for IP ACL middleware with default values.
func DefaultHttpInterIpAclConfig() HttpInterIpAclConfig {
	return HttpInterIpAclConfig{
		HttpInterAclConfig: DefaultHttpInterAclConfig[IpAclRuleConfig](),
	}
}
