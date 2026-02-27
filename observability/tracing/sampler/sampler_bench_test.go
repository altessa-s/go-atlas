// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sampler

import (
	"encoding/binary"
	"testing"
)

func BenchmarkAlwaysOn(b *testing.B) {
	s := AlwaysOn()
	params := SamplingParameters{}
	b.ResetTimer()
	for b.Loop() {
		s.ShouldSample(params)
	}
}

func BenchmarkAlwaysOff(b *testing.B) {
	s := AlwaysOff()
	params := SamplingParameters{}
	b.ResetTimer()
	for b.Loop() {
		s.ShouldSample(params)
	}
}

func BenchmarkTraceIDRatio(b *testing.B) {
	s := NewTraceIDRatio(0.5)
	var traceID [16]byte
	var i int
	b.ResetTimer()
	for b.Loop() {
		binary.BigEndian.PutUint64(traceID[:8], uint64(i))
		s.ShouldSample(SamplingParameters{TraceID: traceID})
		i++
	}
}

func BenchmarkParentBased_WithParent(b *testing.B) {
	s := NewParentBased(AlwaysOn())
	params := SamplingParameters{
		ParentCtx: &SpanContext{
			TraceID:   [16]byte{1},
			SpanID:    [8]byte{1},
			IsSampled: true,
		},
	}
	b.ResetTimer()
	for b.Loop() {
		s.ShouldSample(params)
	}
}

func BenchmarkParentBased_NoParent(b *testing.B) {
	s := NewParentBased(NewTraceIDRatio(0.5))
	var traceID [16]byte
	var i int
	b.ResetTimer()
	for b.Loop() {
		binary.BigEndian.PutUint64(traceID[:8], uint64(i))
		s.ShouldSample(SamplingParameters{TraceID: traceID})
		i++
	}
}
