// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config

import (
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// Auth is the combined authentication and authorization configuration: OIDC
// and mutual-TLS for authentication, OPA and the scope registry for
// authorization. It is an optional convenience grouping — every sub-config is
// still accepted directly by its own factory, so callers that want to wire one
// concern in isolation can keep doing so. A nil sub-config disables that
// concern.
//
// Example:
//
//	auth := &config.Auth{
//		OIDC:  &config.DefaultOIDC(),
//		OPA:   &config.DefaultOPA(),
//		MTLS:  &config.DefaultMTLS(),
//		Scope: &config.DefaultScopeRegistry(),
//	}
type Auth struct {
	// OIDC contains configuration for OpenID Connect authentication.
	// If nil, OIDC authentication is disabled.
	OIDC *OIDC `yaml:"oidc" default:"-"`

	// OPA contains configuration for Open Policy Agent authorization.
	// If nil, OPA authorization is disabled.
	OPA *OPA `yaml:"opa" default:"-"`

	// MTLS contains configuration for mutual-TLS certificate validation.
	// If nil, mTLS peer authentication is disabled.
	MTLS *MTLS `yaml:"mtls" default:"-"`

	// Scope contains the action-key→required-scope registry for scope-based
	// authorization. If nil or empty, scope authorization is disabled.
	Scope *ScopeRegistry `yaml:"scope" default:"-"`

	// Denylist contains the token-revocation denylist configuration. If
	// disabled, token revocation checks are skipped.
	Denylist Denylist `yaml:"denylist"`
}

// DefaultAuth returns an Auth configuration with default values. Every
// sub-config is included with its respective default.
func DefaultAuth() Auth {
	oidc := DefaultOIDC()
	opa := DefaultOPA()
	mtls := DefaultMTLS()
	scope := DefaultScopeRegistry()
	return Auth{
		OIDC:     &oidc,
		OPA:      &opa,
		MTLS:     &mtls,
		Scope:    &scope,
		Denylist: DefaultDenylist(),
	}
}

// Validate validates each present sub-config. A nil sub-config is skipped. An
// empty scope registry is valid (it simply disables scope authorization).
//
// Returns an error if any validation rules fail.
func (a *Auth) Validate() error {
	return ValidateStruct(a,
		validation.Field(&a.OIDC, validation.NilOrNotEmpty),
		validation.Field(&a.OPA, validation.NilOrNotEmpty),
		validation.Field(&a.MTLS, validation.NilOrNotEmpty),
		validation.Field(&a.Scope),
		validation.Field(&a.Denylist),
	)
}

// IsEnabled returns true if any of OIDC, OPA, mTLS, or scope is enabled.
func (a *Auth) IsEnabled() bool {
	return (a.OIDC != nil && a.OIDC.DiscoveryUrl != "") ||
		(a.OPA != nil && a.OPA.IsEnabled()) ||
		(a.MTLS != nil && a.MTLS.IsEnabled()) ||
		(a.Scope != nil && a.Scope.IsEnabled())
}
