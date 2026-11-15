// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"cmp"
	"context"
	"errors"
	"fmt"

	"github.com/altessa-s/go-atlas-core/collections/slices"
	"github.com/altessa-s/go-atlas/observability/tracing/adapters"
	"github.com/altessa-s/go-atlas/observability/tracing/adapters/otlp"

	observabilityconfig "github.com/altessa-s/go-atlas/config/observability"
	tracingfactory "github.com/altessa-s/go-atlas/factory/observability/tracing"
	grpcclientfactory "github.com/altessa-s/go-atlas/factory/transport/grpc/client"
	grpcclient "github.com/altessa-s/go-atlas/transport/grpc/client"
)

// Builder returns the tracing adapter builder of the OTLP type, ready for
// tracingfactory.TracerBuilder.UseAdapter. It reads the otlp block of the
// adapters configuration (the defaults when absent) and builds the exporter's
// client: an SSRF-guarded HTTP client pinned to the collector for
// OTLP/HTTP, a resilient gRPC client for OTLP/gRPC, both with the
// configured proxy policy.
func Builder() tracingfactory.AdapterBuilder {
	return createOTLPAdapter
}

// createOTLPAdapter creates an OTLP adapter from the adapters block.
func createOTLPAdapter(ctx context.Context, adaptersCfg *observabilityconfig.TracingAdapters) (adapters.Adapter, error) {
	var cfg *observabilityconfig.TracingOTLP
	if adaptersCfg != nil {
		cfg = adaptersCfg.OTLP
	}
	if cfg == nil {
		def := observabilityconfig.DefaultTracingOTLP()
		cfg = &def
	}

	opts := []otlp.Option{
		otlp.WithEndpoint(cfg.Endpoint),
		otlp.WithProtocol(otlp.Protocol(cmp.Or(cfg.Protocol, observabilityconfig.OTLPProtocolGRPC))),
		otlp.WithCompression(cfg.Compression),
	}
	opts = slices.AppendIf(opts, cfg.Insecure, otlp.WithInsecure())
	opts = slices.AppendIf(opts, len(cfg.Headers) > 0, otlp.WithHeaders(cfg.Headers))

	if cfg.Protocol == observabilityconfig.OTLPProtocolHTTP {
		client, err := newOTLPHTTPClient(cfg)
		if err != nil {
			return nil, fmt.Errorf("create OTLP HTTP client: %w", err)
		}
		return otlp.New(ctx, append(opts, otlp.WithHTTPClient(client))...)
	}
	return createOTLPGRPCAdapter(ctx, cfg, opts)
}

// createOTLPGRPCAdapter builds the resilient gRPC client of the OTLP/gRPC
// exporter with the configured proxy policy and hands its connection to
// the adapter. The returned adapter closes the client on Shutdown.
func createOTLPGRPCAdapter(ctx context.Context, cfg *observabilityconfig.TracingOTLP, opts []otlp.Option) (adapters.Adapter, error) {
	proxyOpts, err := grpcclientfactory.ProxyOptions(cfg.Proxy)
	if err != nil {
		return nil, fmt.Errorf("failed to materialize OTLP proxy options: %w", err)
	}
	grpcOpts := slices.AppendIf([]grpcclient.Option(nil), cfg.Insecure, grpcclient.WithInsecure())
	client, err := grpcclient.New(ctx, cfg.Endpoint, append(grpcOpts, proxyOpts...)...)
	if err != nil {
		return nil, fmt.Errorf("create OTLP gRPC client: %w", err)
	}
	conn, err := client.GetConnection(ctx)
	if err != nil {
		return nil, errors.Join(fmt.Errorf("get OTLP gRPC connection: %w", err), client.Close(ctx))
	}
	adapter, err := otlp.New(ctx, append(opts, otlp.WithGRPCConn(conn))...)
	if err != nil {
		return nil, errors.Join(err, client.Close(ctx))
	}
	return &ownedConnAdapter{Adapter: adapter, client: client}, nil
}

// ownedConnAdapter closes the gRPC client the builder made for the OTLP
// adapter once the adapter has shut down: the adapter leaves an injected
// connection to its owner.
type ownedConnAdapter struct {
	adapters.Adapter
	client *grpcclient.Client
}

// Shutdown stops the adapter, then closes its gRPC client.
func (a *ownedConnAdapter) Shutdown(ctx context.Context) error {
	return errors.Join(a.Adapter.Shutdown(ctx), a.client.Close(ctx))
}
