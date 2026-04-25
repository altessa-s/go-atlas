// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sampler

import "fmt"

// parentBasedSampler defers to the parent span's sampling decision.
// If there is no parent, it uses a root sampler.
type parentBasedSampler struct {
	root                   Sampler
	remoteParentSampled    Sampler
	remoteParentNotSampled Sampler
	localParentSampled     Sampler
	localParentNotSampled  Sampler
	description            string
}

// ParentBasedOption configures the parent-based sampler.
type ParentBasedOption func(*parentBasedSampler)

// NewParentBased returns a sampler that defers to the parent span's decision.
// If there is no parent span (root span), it uses the provided root sampler.
//
// By default:
//   - If parent is remote and sampled: sample
//   - If parent is remote and not sampled: don't sample
//   - If parent is local and sampled: sample
//   - If parent is local and not sampled: don't sample
//   - If no parent (root): use root sampler
//
// Example:
//
//	// Use TraceIDRatio for root spans, follow parent for child spans
//	sampler := sampler.NewParentBased(
//	    sampler.NewTraceIDRatio(0.1),
//	)
//
//	// Custom behavior for remote parents
//	sampler := sampler.NewParentBased(
//	    sampler.NewTraceIDRatio(0.1),
//	    sampler.WithRemoteParentSampled(sampler.AlwaysOn()),
//	)
func NewParentBased(root Sampler, opts ...ParentBasedOption) Sampler {
	s := &parentBasedSampler{
		root:                   root,
		remoteParentSampled:    AlwaysOn(),
		remoteParentNotSampled: AlwaysOff(),
		localParentSampled:     AlwaysOn(),
		localParentNotSampled:  AlwaysOff(),
	}

	for _, opt := range opts {
		opt(s)
	}

	s.description = fmt.Sprintf("ParentBased{root:%s}", root.Description())
	return s
}

// WithRemoteParentSampled sets the sampler for remote sampled parents.
func WithRemoteParentSampled(s Sampler) ParentBasedOption {
	return func(p *parentBasedSampler) {
		p.remoteParentSampled = s
	}
}

// WithRemoteParentNotSampled sets the sampler for remote not-sampled parents.
func WithRemoteParentNotSampled(s Sampler) ParentBasedOption {
	return func(p *parentBasedSampler) {
		p.remoteParentNotSampled = s
	}
}

// WithLocalParentSampled sets the sampler for local sampled parents.
func WithLocalParentSampled(s Sampler) ParentBasedOption {
	return func(p *parentBasedSampler) {
		p.localParentSampled = s
	}
}

// WithLocalParentNotSampled sets the sampler for local not-sampled parents.
func WithLocalParentNotSampled(s Sampler) ParentBasedOption {
	return func(p *parentBasedSampler) {
		p.localParentNotSampled = s
	}
}

// ShouldSample implements Sampler.
func (s *parentBasedSampler) ShouldSample(params SamplingParameters) SamplingResult {
	parent := params.ParentCtx

	// No parent - use root sampler
	if parent == nil || !parent.IsValid() {
		return s.root.ShouldSample(params)
	}

	// Has parent - defer to parent's decision
	if parent.IsRemote {
		if parent.IsSampled {
			return s.remoteParentSampled.ShouldSample(params)
		}
		return s.remoteParentNotSampled.ShouldSample(params)
	}

	// Local parent
	if parent.IsSampled {
		return s.localParentSampled.ShouldSample(params)
	}
	return s.localParentNotSampled.ShouldSample(params)
}

// Description implements Sampler.
func (s *parentBasedSampler) Description() string {
	return s.description
}

// Ensure implementation satisfies the interface.
var _ Sampler = (*parentBasedSampler)(nil)
