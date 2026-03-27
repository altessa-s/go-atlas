// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config

// Security headers configuration defaults.
const (
	// DefaultHstsMaxAge is the default max-age for HSTS (1 year in seconds).
	DefaultHstsMaxAge = 31536000
)

// HttpInterSecurityHeadersConfig defines the configuration for HTTP security headers middleware.
type HttpInterSecurityHeadersConfig struct {
	// BaseHttpMiddlewareConfig provides standard enable and filtering fields.
	BaseHttpMiddlewareConfig `yaml:",inline"`

	// FrameOptions sets the X-Frame-Options header.
	// Common values: "DENY", "SAMEORIGIN".
	FrameOptions string `yaml:"frameOptions" default:"DENY"`

	// ReferrerPolicy sets the Referrer-Policy header.
	// Common values: "strict-origin-when-cross-origin", "no-referrer".
	ReferrerPolicy string `yaml:"referrerPolicy" default:"strict-origin-when-cross-origin"`

	// ContentSecurityPolicy sets the Content-Security-Policy header.
	ContentSecurityPolicy string `yaml:"contentSecurityPolicy"`

	// PermissionsPolicy sets the Permissions-Policy header.
	PermissionsPolicy string `yaml:"permissionsPolicy"`

	// HstsEnabled enables the Strict-Transport-Security (HSTS) header.
	HstsEnabled bool `yaml:"hstsEnabled" default:"false"`

	// HstsMaxAge sets the max-age value for HSTS.
	HstsMaxAge int `yaml:"hstsMaxAge" default:"31536000"`

	// HstsIncludeSubDomains includes subdomains in HSTS policy.
	HstsIncludeSubDomains bool `yaml:"hstsIncludeSubDomains" default:"true"`

	// HstsPreload enables the HSTS preload directive.
	HstsPreload bool `yaml:"hstsPreload" default:"false"`

	// ContentTypeNoSniff sets X-Content-Type-Options: nosniff.
	ContentTypeNoSniff bool `yaml:"contentTypeNoSniff" default:"true"`

	// XssProtectionDisabled sets X-XSS-Protection: 0.
	XssProtectionDisabled bool `yaml:"xssProtectionDisabled" default:"true"`
}

// Validate performs validation of the HttpInterSecurityHeadersConfig.
func (c *HttpInterSecurityHeadersConfig) Validate() error {
	return c.ValidateBase()
}

// DefaultHttpInterSecurityHeadersConfig returns a configuration for securityheaders middleware with default values.
func DefaultHttpInterSecurityHeadersConfig() HttpInterSecurityHeadersConfig {
	return HttpInterSecurityHeadersConfig{
		BaseHttpMiddlewareConfig: BaseHttpMiddlewareConfig{
			EnableMixin: EnableMixin{Enabled: false},
		},
		FrameOptions:          "DENY",
		ReferrerPolicy:        "strict-origin-when-cross-origin",
		HstsMaxAge:            DefaultHstsMaxAge,
		HstsIncludeSubDomains: true,
		ContentTypeNoSniff:    true,
		XssProtectionDisabled: true,
	}
}

// IsEnabled returns true if the middleware is enabled.
func (c *HttpInterSecurityHeadersConfig) IsEnabled() bool {
	return c != nil && c.Enabled
}
