// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package otlp provides an OTLP [adapters.Adapter] for tracing.
// It exports [adapters.SpanData] using the OpenTelemetry Protocol (OTLP) over gRPC
// or HTTP.
//
// # Outbound clients
//
// The exporter sends on an injected client: [WithGRPCConn] for gRPC (owned by
// the caller, left open by Shutdown) or [WithHTTPClient] for HTTP. Without
// one, the gRPC exporter dials the endpoint with grpc-go defaults and closes
// that connection on Shutdown, and the HTTP exporter uses the otlptracehttp
// default client; both honor HTTPS_PROXY, HTTP_PROXY and NO_PROXY. The
// tracing factory injects transport/grpc/client and net/http/client
// clients carrying the configured proxy policy.
//
// # Example
//
//	adapter, err := otlp.New(ctx,
//	    otlp.WithEndpoint("localhost:4317"),
//	    otlp.WithInsecure(),
//	)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer adapter.Shutdown(context.Background())
//
//	provider := tracing.New(tracing.WithAdapter(adapter))
package otlp
