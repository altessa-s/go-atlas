// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config

import (
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// Auth represents the combined authentication and authorization configuration.
// It includes settings for OIDC (Authentication) and OPA (Authorization).
//
// Example:
//
//	auth := &config.Auth{
//		OIDC: &config.DefaultOIDC(),
//		OPA:  &config.DefaultOPA(),
//	}
type Auth struct {
	// OIDC contains configuration for OpenID Connect authentication.
	// If nil, OIDC authentication is disabled.
	OIDC *OIDC `yaml:"oidc" default:"-"`

	// OPA contains configuration for Open Policy Agent authorization.
	// If nil, OPA authorization is disabled.
	OPA *OPA `yaml:"opa" default:"-"`
}

// DefaultAuth returns an Auth configuration with default values.
// Both OIDC and OPA are included with their respective default values.
func DefaultAuth() Auth {
	oidc := DefaultOIDC()
	opa := DefaultOPA()
	return Auth{
		OIDC: &oidc,
		OPA:  &opa,
	}
}

// Validate validates the Auth configuration.
// It ensures that both OIDC and OPA configurations are valid if present.
//
// Returns an error if any validation rules fail.
func (a *Auth) Validate() error {
	return ValidateStruct(a,
		validation.Field(&a.OIDC, validation.NilOrNotEmpty),
		validation.Field(&a.OPA, validation.NilOrNotEmpty),
	)
}

// IsEnabled returns true if either OIDC or OPA is enabled.
func (a *Auth) IsEnabled() bool {
	return (a.OIDC != nil && a.OIDC.DiscoveryUrl != "") || (a.OPA != nil && a.OPA.IsEnabled())
}
