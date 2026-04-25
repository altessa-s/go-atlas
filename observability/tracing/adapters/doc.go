// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package adapters defines the [Adapter] interface for tracing backends.
// Adapters translate abstract [SpanData] to specific backend formats
// such as OTLP, Jaeger, Zipkin, or console output.
//
// Use [MultiAdapter] to broadcast to multiple backends simultaneously.
// Implementations must be safe for concurrent use.
//
// # Example
//
//	type MyAdapter struct{}
//
//	func (a *MyAdapter) Name() string { return "myadapter" }
//	func (a *MyAdapter) ExportSpans(ctx context.Context, spans []SpanData) error { return nil }
//	func (a *MyAdapter) Shutdown(ctx context.Context) error { return nil }
//	func (a *MyAdapter) ForceFlush(ctx context.Context) error { return nil }
package adapters
