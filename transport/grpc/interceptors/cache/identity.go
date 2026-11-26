// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package cache

import (
	"context"
	"slices"

	"github.com/altessa-s/go-atlas-core/encoding/keys"
	"github.com/altessa-s/go-atlas/auth/oidc"
	"github.com/altessa-s/go-atlas/auth/principal"
	"github.com/altessa-s/go-atlas/internal/transport/authprincipal"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/auth"
)

// IdentityState describes whether a request can be assigned a cache namespace.
type IdentityState uint8

const (
	// AuthenticatedUnknown bypasses caching when authentication did not yield an identifiable caller.
	AuthenticatedUnknown IdentityState = iota
	// Verified permits caching in the nonempty namespace of a verified caller.
	Verified
	// NoAuth permits shared caching only for explicitly public methods in a chain without required auth.
	NoAuth
)

// IdentityFunc resolves the cache namespace from verified context data.
// A Verified namespace must be stable, nonempty, and distinguish tenants,
// subjects, issuers, and every grant or claim that can change the response.
// Return [AuthenticatedUnknown] if a safe namespace cannot be resolved.
type IdentityFunc func(context.Context) (namespace string, state IdentityState)

func defaultIdentity(ctx context.Context, authRequired bool) (string, IdentityState) {
	creds, authenticated := auth.CredentialsFromContext(ctx)
	if authenticated {
		// Shadow any upstream principal before resolving the current credentials.
		ctx = authprincipal.Bridge(principal.NewContext(ctx, principal.Principal{}), creds.Data)
	} else if authRequired {
		return "", AuthenticatedUnknown
	}

	p, ok := principal.FromContext(ctx)
	if !ok && !authenticated {
		return "", NoAuth
	}
	if p.Subject == "" {
		return "", AuthenticatedUnknown
	}

	scopes := slices.Clone(p.Scopes)
	roles := slices.Clone(p.Roles)
	slices.Sort(scopes)
	slices.Sort(roles)
	return keys.Compose(string(creds.AuthMethod), verifiedIssuer(p, creds.Data), p.Tenant, p.Subject,
		keys.Compose(scopes...), keys.Compose(roles...)), Verified
}

func verifiedIssuer(p principal.Principal, data any) string {
	switch claims := data.(type) {
	case *oidc.Claims:
		return claims.Issuer
	case interface{ Issuer() string }:
		return claims.Issuer()
	default:
		issuer, _ := p.Claims["iss"].(string)
		return issuer
	}
}

func cacheNamespace(ctx context.Context, identity IdentityFunc, authRequired, public bool) (string, bool) {
	var namespace string
	var state IdentityState
	if identity == nil {
		namespace, state = defaultIdentity(ctx, authRequired)
	} else {
		namespace, state = identity(ctx)
	}
	switch state {
	case Verified:
		return namespace, namespace != ""
	case NoAuth:
		return "", !authRequired && public
	default:
		return "", false
	}
}
