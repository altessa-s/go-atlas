// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"context"
	"errors"
	"fmt"

	"github.com/spiffe/go-spiffe/v2/spiffeid"
	"github.com/spiffe/go-spiffe/v2/spiffetls/tlsconfig"

	"github.com/altessa-s/go-atlas/config"

	coreslices "github.com/altessa-s/go-atlas/core/collections/slices"
	coreerrs "github.com/altessa-s/go-atlas/core/errors"
	corespiffe "github.com/altessa-s/go-atlas/security/tlsutils/spiffe"
)

var (
	// ErrNoConfig indicates a nil [config.SPIFFE] was passed to [New].
	ErrNoConfig = errors.New("spiffe/factory: nil config")
	// ErrNoAuthorizerSource indicates the configuration lists no trust domains
	// and no IDs, so no peer authorizer can be derived. The stack is
	// fail-closed and refuses to trust every peer.
	ErrNoAuthorizerSource = errors.New("spiffe/factory: no allowed trust domains or ids")
)

// Builder turns a [config.SPIFFE] into the options and provider for the core
// spiffe package. It mirrors the factory pattern used across the auth stack:
// configuration in, opinionated construction out.
type Builder struct {
	cfg *config.SPIFFE
}

// New builds a Builder for the given configuration.
func New(cfg *config.SPIFFE) *Builder {
	return &Builder{cfg: cfg}
}

// Authorizer derives the peer authorizer from the configured trust domains and
// IDs. A peer is accepted when it belongs to any allowed trust domain or
// matches any allowed ID. With neither configured it fails with
// [ErrNoAuthorizerSource].
func (b *Builder) Authorizer() (tlsconfig.Authorizer, error) {
	if b.cfg == nil {
		return nil, ErrNoConfig
	}
	var matchers []spiffeid.Matcher
	if len(b.cfg.AllowedIDs) > 0 {
		ids := make([]spiffeid.ID, 0, len(b.cfg.AllowedIDs))
		for _, raw := range b.cfg.AllowedIDs {
			id, err := spiffeid.FromString(raw)
			if err != nil {
				return nil, coreerrs.Wrap(err, "spiffe/factory: parse allowed id")
			}
			ids = append(ids, id)
		}
		matchers = append(matchers, spiffeid.MatchOneOf(ids...))
	}
	for _, raw := range b.cfg.AllowedTrustDomains {
		td, err := spiffeid.TrustDomainFromString(raw)
		if err != nil {
			return nil, coreerrs.Wrap(err, "spiffe/factory: parse trust domain")
		}
		matchers = append(matchers, spiffeid.MatchMemberOf(td))
	}
	if len(matchers) == 0 {
		return nil, ErrNoAuthorizerSource
	}
	return tlsconfig.AdaptMatcher(matchAnyOf(matchers...)), nil
}

// Options returns the core spiffe options derived from the configuration: the
// authorizer and, when set, the Workload API socket path.
func (b *Builder) Options() ([]corespiffe.Option, error) {
	auth, err := b.Authorizer()
	if err != nil {
		return nil, err
	}
	opts := []corespiffe.Option{corespiffe.WithAuthorizer(auth)}
	opts = coreslices.AppendIf(opts, b.cfg.SocketPath != "", corespiffe.WithSocketPath(b.cfg.SocketPath))
	return opts, nil
}

// Provider builds a connected [corespiffe.Provider] from the configuration,
// appending any extra options. It blocks until the first SVID is received.
func (b *Builder) Provider(ctx context.Context, extra ...corespiffe.Option) (*corespiffe.Provider, error) {
	opts, err := b.Options()
	if err != nil {
		return nil, err
	}
	return corespiffe.New(ctx, append(opts, extra...)...)
}

// matchAnyOf accepts an ID when any of the matchers accepts it.
func matchAnyOf(matchers ...spiffeid.Matcher) spiffeid.Matcher {
	return func(id spiffeid.ID) error {
		for _, m := range matchers {
			if m(id) == nil {
				return nil
			}
		}
		return fmt.Errorf("spiffe: peer id %q is not authorized", id)
	}
}
