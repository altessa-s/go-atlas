// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package sampler provides sampling strategies for distributed tracing.
// A [Sampler] decides whether a trace should be recorded and exported.
//
// # Available Samplers
//
//   - [AlwaysOn]: Samples all traces.
//   - [AlwaysOff]: Samples no traces.
//   - [NewTraceIDRatio]: Samples a deterministic percentage based on trace ID.
//   - [NewParentBased]: Defers to the parent span's sampling decision.
//
// # Usage
//
//	// Sample 10% of traces
//	s := sampler.NewTraceIDRatio(0.1)
//
//	// Use parent's decision, with fallback to 10% for root spans
//	s := sampler.NewParentBased(
//	    sampler.NewTraceIDRatio(0.1),
//	)
//
// # Custom Samplers
//
// Implement the [Sampler] interface for custom sampling logic:
//
//	type MySampler struct{}
//
//	func (s *MySampler) ShouldSample(params SamplingParameters) SamplingResult {
//	    // Custom logic
//	    return SamplingResult{Decision: RecordAndSample}
//	}
//
//	func (s *MySampler) Description() string {
//	    return "MySampler"
//	}
package sampler
