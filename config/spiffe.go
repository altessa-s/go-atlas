// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config

import (
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// SPIFFE configures the SPIFFE Workload API source built by
// [security/tlsutils/spiffe/factory.New]: the Workload API endpoint plus the
// accepted peer identities that the derived authorizer enforces. Identity and
// rotation are handled by the source; this only drives where credentials come
// from and which peers are trusted.
//
// At least one of AllowedTrustDomains or AllowedIDs must be set — the source is
// fail-closed and refuses to build an authorizer that trusts every peer.
//
// Example:
//
//	spiffe:
//	  socketPath: "unix:///run/spire/agent/api.sock"
//	  allowedTrustDomains: ["example.org"]
//	  allowedIDs: []
type SPIFFE struct {
	// SocketPath is the Workload API endpoint address. Empty falls back to the
	// standard SPIFFE_ENDPOINT_SOCKET environment variable.
	SocketPath string `yaml:"socketPath"`

	// AllowedTrustDomains lists the SPIFFE trust domains whose members are
	// accepted as peers.
	AllowedTrustDomains []string `yaml:"allowedTrustDomains"`

	// AllowedIDs lists exact SPIFFE IDs accepted as peers, in addition to any
	// AllowedTrustDomains.
	AllowedIDs []string `yaml:"allowedIDs"`
}

// Validate validates the SPIFFE configuration.
func (c *SPIFFE) Validate() error {
	return ValidateStruct(c,
		validation.Field(&c.AllowedTrustDomains,
			validation.Required.When(len(c.AllowedIDs) == 0).
				Error("either allowedTrustDomains or allowedIDs must be set")),
	)
}

// DefaultSPIFFE returns a SPIFFE configuration with default values.
func DefaultSPIFFE() SPIFFE {
	return SPIFFE{}
}

// IsEnabled reports whether the configuration is present.
func (c *SPIFFE) IsEnabled() bool {
	return c != nil
}
