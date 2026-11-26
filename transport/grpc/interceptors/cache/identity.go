// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package cache

import (
	"context"

	"github.com/altessa-s/go-atlas-core/encoding/keys"
	"github.com/altessa-s/go-atlas/auth/principal"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/auth"
)

// IdentityState describes whether a request can be assigned a cache namespace.
type IdentityState uint8

const (
	// AuthenticatedUnknown bypasses caching when authentication did not yield an identifiable caller.
	AuthenticatedUnknown IdentityState = iota
	// Verified permits caching in the nonempty namespace of a verified caller.
	Verified
	// NoAuth permits shared caching when the service does not authenticate the request.
	NoAuth
)

// IdentityFunc resolves the cache namespace from verified context data.
// Use a stable namespace that includes the issuer if multiple issuers can
// assign the same tenant and subject. Return [AuthenticatedUnknown] when no
// safe namespace can be resolved.
type IdentityFunc func(context.Context) (namespace string, state IdentityState)

func defaultIdentity(ctx context.Context) (string, IdentityState) {
	if p, ok := principal.FromContext(ctx); ok {
		if p.Subject == "" {
			return "", AuthenticatedUnknown
		}
		return keys.Compose(p.Tenant, p.Subject), Verified
	}

	if _, ok := auth.CredentialsFromContext(ctx); ok {
		return "", AuthenticatedUnknown
	}
	return "", NoAuth
}

func cacheNamespace(ctx context.Context, identity IdentityFunc, authRequired bool) (string, bool) {
	namespace, state := identity(ctx)
	switch state {
	case Verified:
		return namespace, namespace != ""
	case NoAuth:
		return "", !authRequired
	default:
		return "", false
	}
}
