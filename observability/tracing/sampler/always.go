// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sampler

// alwaysOnSampler samples all traces.
type alwaysOnSampler struct{}

// alwaysOnSamplerInstance is the singleton instance.
var alwaysOnSamplerInstance = &alwaysOnSampler{}

// AlwaysOn returns a sampler that samples all traces.
// This is useful for development or when you want to capture all traces.
//
// Example:
//
//	sampler := sampler.AlwaysOn()
func AlwaysOn() Sampler {
	return alwaysOnSamplerInstance
}

// ShouldSample implements Sampler.
func (s *alwaysOnSampler) ShouldSample(_ SamplingParameters) SamplingResult {
	return SamplingResult{Decision: RecordAndSample}
}

// Description implements Sampler.
func (s *alwaysOnSampler) Description() string {
	return "AlwaysOnSampler"
}

// alwaysOffSampler never samples traces.
type alwaysOffSampler struct{}

// alwaysOffSamplerInstance is the singleton instance.
var alwaysOffSamplerInstance = &alwaysOffSampler{}

// AlwaysOff returns a sampler that never samples traces.
// This effectively disables tracing while still allowing span creation.
//
// Example:
//
//	sampler := sampler.AlwaysOff()
func AlwaysOff() Sampler {
	return alwaysOffSamplerInstance
}

// ShouldSample implements Sampler.
func (s *alwaysOffSampler) ShouldSample(_ SamplingParameters) SamplingResult {
	return SamplingResult{Decision: Drop}
}

// Description implements Sampler.
func (s *alwaysOffSampler) Description() string {
	return "AlwaysOffSampler"
}

// Ensure implementations satisfy the interface.
var (
	_ Sampler = (*alwaysOnSampler)(nil)
	_ Sampler = (*alwaysOffSampler)(nil)
)
