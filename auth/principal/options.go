// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package principal

//go:generate go run github.com/altessa-s/go-atlas/cmd/optgen generate

// Default claim names used by [FromClaims] when not overridden.
const (
	// DefaultTenantClaim is the claim [FromClaims] reads into [Principal.Tenant].
	DefaultTenantClaim = "tenant"
	// DefaultRolesClaim is the claim [FromClaims] reads into [Principal.Roles].
	DefaultRolesClaim = "roles"
)

// options configures the claim-name mapping applied by [FromClaims].
type options struct {
	tenantClaim string `optgen:"default=DefaultTenantClaim"`
	rolesClaim  string `optgen:"default=DefaultRolesClaim"`
}
