// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package httpconfig

import middlewareconfig "github.com/altessa-s/go-atlas/config/middleware"

// GeoACLMiddleware defines the configuration for the HTTP geographic access control middleware.
type GeoACLMiddleware struct {
	ACLMiddleware[middlewareconfig.GeoACLRule] `yaml:",inline"`
}

// Validate performs validation of the HTTP geographic access control middleware configuration.
func (c *GeoACLMiddleware) Validate() error {
	return c.ValidateWith(middlewareconfig.ValidateGeoACLRule)
}

// DefaultGeoACLMiddleware returns a configuration for GeoACL middleware with default values.
func DefaultGeoACLMiddleware() GeoACLMiddleware {
	return GeoACLMiddleware{
		ACLMiddleware: DefaultACLMiddleware[middlewareconfig.GeoACLRule](),
	}
}
