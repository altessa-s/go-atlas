// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package tracing

import "github.com/altessa-s/go-atlas/observability/internal/shared"

// scopedTracer is a Tracer that adds a scope prefix to all recorders.
type scopedTracer struct {
	parent *tracer
	scope  string
}

// Recorder implements Tracer.
func (s *scopedTracer) Recorder(name string, opts ...RecorderOption) Recorder {
	return s.parent.getOrCreateRecorder(s.scope, name, opts...)
}

// WithScope implements Tracer.
// Nested scopes are joined with slashes.
func (s *scopedTracer) WithScope(scope string) Tracer {
	return &scopedTracer{
		parent: s.parent,
		scope:  shared.JoinTracerScope(s.scope, scope),
	}
}

// Ensure scopedTracer implements Tracer.
var _ Tracer = (*scopedTracer)(nil)
