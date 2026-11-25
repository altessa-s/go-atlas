// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"github.com/altessa-s/go-atlas-core/collections/slices"
	"github.com/altessa-s/go-atlas/transport/grpc/client/dnsresolver"

	grpcclientconfig "github.com/altessa-s/go-atlas/config/grpcclient"
	grpcclient "github.com/altessa-s/go-atlas/transport/grpc/client"
)

// ConnectionOptions returns gRPC client options for name resolution and
// connection backoff. Zero fields and unset jitters keep the client
// defaults, and a nil cfg returns nil. Validate the schema before assembly.
//
// Example:
//
//	client, err := grpcclient.New(ctx, target, append(
//	    []grpcclient.Option{grpcclient.WithLogger(logger)},
//	    factory.ConnectionOptions(cfg.GRPCClient)...,
//	)...)
func ConnectionOptions(cfg *grpcclientconfig.Config) []grpcclient.Option {
	if cfg == nil {
		return nil
	}

	var opts []grpcclient.Option
	if cfg.Resolver.Enabled {
		opts = append(opts, grpcclient.WithDNSResolver(resolverOptions(&cfg.Resolver)...))
	}
	if backoffOpts := connectBackoffOptions(&cfg.ConnectBackoff); len(backoffOpts) > 0 {
		opts = append(opts, grpcclient.WithConnectBackoff(backoffOpts...))
	}
	return opts
}

// resolverOptions maps the set resolver fields to resolver options.
func resolverOptions(r *grpcclientconfig.Resolver) []dnsresolver.Option {
	var opts []dnsresolver.Option
	opts = slices.AppendIf(opts, r.RefreshInterval > 0, dnsresolver.WithRefreshInterval(r.RefreshInterval))
	opts = slices.AppendIfFunc(opts, r.RefreshJitter != nil, func() []dnsresolver.Option {
		return []dnsresolver.Option{dnsresolver.WithRefreshJitter(*r.RefreshJitter)}
	})
	opts = slices.AppendIf(opts, r.MinInterval > 0, dnsresolver.WithMinInterval(r.MinInterval))
	opts = slices.AppendIf(opts, r.LookupTimeout > 0, dnsresolver.WithLookupTimeout(r.LookupTimeout))
	opts = slices.AppendIf(opts, r.Backoff.BaseDelay > 0, dnsresolver.WithBackoffBaseDelay(r.Backoff.BaseDelay))
	opts = slices.AppendIf(opts, r.Backoff.Multiplier > 0, dnsresolver.WithBackoffMultiplier(r.Backoff.Multiplier))
	opts = slices.AppendIfFunc(opts, r.Backoff.Jitter != nil, func() []dnsresolver.Option {
		return []dnsresolver.Option{dnsresolver.WithBackoffJitter(*r.Backoff.Jitter)}
	})
	opts = slices.AppendIf(opts, r.Backoff.MaxDelay > 0, dnsresolver.WithBackoffMaxDelay(r.Backoff.MaxDelay))
	return opts
}

// connectBackoffOptions maps the set connection backoff fields to options.
func connectBackoffOptions(b *grpcclientconfig.ConnectBackoff) []grpcclient.ConnectBackoffOption {
	var opts []grpcclient.ConnectBackoffOption
	opts = slices.AppendIf(opts, b.BaseDelay > 0, grpcclient.WithConnectBackoffBaseDelay(b.BaseDelay))
	opts = slices.AppendIf(opts, b.Multiplier > 0, grpcclient.WithConnectBackoffMultiplier(b.Multiplier))
	opts = slices.AppendIfFunc(opts, b.Jitter != nil, func() []grpcclient.ConnectBackoffOption {
		return []grpcclient.ConnectBackoffOption{grpcclient.WithConnectBackoffJitter(*b.Jitter)}
	})
	opts = slices.AppendIf(opts, b.MaxDelay > 0, grpcclient.WithConnectBackoffMaxDelay(b.MaxDelay))
	return opts
}
