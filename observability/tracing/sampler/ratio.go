// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sampler

import (
	"encoding/binary"
	"fmt"
	"math"
)

// traceIDRatioSampler samples a fraction of traces based on trace ID.
// It uses a deterministic algorithm so the same trace ID always produces
// the same sampling decision.
type traceIDRatioSampler struct {
	ratio       float64
	description string
	upperBound  uint64
}

// NewTraceIDRatio returns a sampler that samples a fraction of traces.
// The ratio must be between 0.0 and 1.0, where:
//   - 0.0 means no traces are sampled (equivalent to AlwaysOff)
//   - 1.0 means all traces are sampled (equivalent to AlwaysOn)
//   - 0.5 means approximately 50% of traces are sampled
//
// The sampling decision is deterministic based on the trace ID,
// so the same trace ID will always produce the same decision.
//
// Example:
//
//	// Sample 10% of traces
//	sampler := sampler.NewTraceIDRatio(0.1)
//
//	// Sample 1% of traces
//	sampler := sampler.NewTraceIDRatio(0.01)
func NewTraceIDRatio(ratio float64) Sampler {
	// Clamp ratio to valid range
	if ratio < 0.0 {
		ratio = 0.0
	}
	if ratio > 1.0 {
		ratio = 1.0
	}

	// Fast path for edge cases
	if ratio == 0.0 {
		return AlwaysOff()
	}
	if ratio >= 1.0 {
		return AlwaysOn()
	}

	// Calculate the upper bound for sampling.
	// We use the first 8 bytes of the trace ID as a uint64 and compare
	// it to the upper bound. If the value is less than the upper bound,
	// the trace is sampled.
	upperBound := uint64(ratio * float64(math.MaxUint64))

	return &traceIDRatioSampler{
		ratio:       ratio,
		description: fmt.Sprintf("TraceIDRatioBased{%.6f}", ratio),
		upperBound:  upperBound,
	}
}

// ShouldSample implements Sampler.
func (s *traceIDRatioSampler) ShouldSample(params SamplingParameters) SamplingResult {
	// Use the first 8 bytes of the trace ID for sampling decision
	x := binary.BigEndian.Uint64(params.TraceID[:8])

	if x < s.upperBound {
		return SamplingResult{Decision: RecordAndSample}
	}
	return SamplingResult{Decision: Drop}
}

// Description implements Sampler.
func (s *traceIDRatioSampler) Description() string {
	return s.description
}

// Ratio returns the sampling ratio.
func (s *traceIDRatioSampler) Ratio() float64 {
	return s.ratio
}

// Ensure implementation satisfies the interface.
var _ Sampler = (*traceIDRatioSampler)(nil)
