// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package spiffe

//go:generate go run github.com/altessa-s/go-atlas/cmd/optgen generate

import "github.com/spiffe/go-spiffe/v2/spiffetls/tlsconfig"

type options struct {
	// socketPath is the SPIFFE Workload API endpoint address (for example
	// "unix:///run/spire/agent/api.sock"). Empty falls back to the standard
	// SPIFFE_ENDPOINT_SOCKET environment variable.
	socketPath string

	// authorizer matches the accepted peer SPIFFE IDs. Set through the
	// hand-written WithAuthorizer; no default, so the provider is fail-closed.
	authorizer tlsconfig.Authorizer `opt:"-"`
}

// WithAuthorizer sets the peer authorizer used to verify the remote SPIFFE ID
// during the handshake. It is mandatory — [New] fails with [ErrNoAuthorizer]
// when none is supplied. Build one with the helpers in
// [github.com/spiffe/go-spiffe/v2/spiffetls/tlsconfig] (AuthorizeMemberOf,
// AuthorizeOneOf, AuthorizeID, AdaptMatcher), or use the factory to derive it
// from configuration. A nil authorizer is ignored.
func WithAuthorizer(a tlsconfig.Authorizer) Option {
	return func(o *options) {
		if a == nil {
			return
		}
		o.authorizer = a
	}
}
