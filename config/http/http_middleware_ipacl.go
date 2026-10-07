// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package httpconfig

import middlewareconfig "github.com/altessa-s/go-atlas/config/middleware"

// IPACLMiddleware defines the configuration for the HTTP IP access control middleware.
type IPACLMiddleware struct {
	ACLMiddleware[middlewareconfig.IPACLRule] `yaml:",inline"`
}

// Validate performs validation of the HTTP IP access control middleware configuration.
func (c *IPACLMiddleware) Validate() error {
	return c.ValidateWith(middlewareconfig.ValidateIPACLRule)
}

// DefaultIPACLMiddleware returns a configuration for IP ACL middleware with default values.
func DefaultIPACLMiddleware() IPACLMiddleware {
	return IPACLMiddleware{
		ACLMiddleware: DefaultACLMiddleware[middlewareconfig.IPACLRule](),
	}
}
