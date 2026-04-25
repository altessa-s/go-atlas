// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package otlp provides an OTLP [adapters.Adapter] for tracing.
// It exports [adapters.SpanData] using the OpenTelemetry Protocol (OTLP) over gRPC.
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
