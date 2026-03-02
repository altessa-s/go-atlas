// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package tracing

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
)

// randBufSize is the size of the buffered CSPRNG pool. Each refill is a single
// syscall to crypto/rand; between refills the bytes are served from memory.
const randBufSize = 4096

// randPool amortises crypto/rand syscall overhead by reading in bulk.
type randPool struct {
	mu  sync.Mutex
	buf [randBufSize]byte
	pos int
}

// rng is the package-level buffered random source.
// pos starts at randBufSize to force an immediate fill on first use.
var rng = &randPool{pos: randBufSize}

// read copies len(dst) cryptographically-secure random bytes into dst.
func (r *randPool) read(dst []byte) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.pos+len(dst) > randBufSize {
		_, _ = rand.Read(r.buf[:]) //nolint:errcheck // only fails on reader exhaustion
		r.pos = 0
	}
	copy(dst, r.buf[r.pos:r.pos+len(dst)])
	r.pos += len(dst)
}

// spanContextImpl is the concrete implementation of SpanContext.
type spanContextImpl struct {
	traceID    [16]byte
	spanID     [8]byte
	traceFlags TraceFlags
	remote     bool
}

// newSpanContextImpl creates a new SpanContext.
// If parent is valid, the trace ID is inherited.
func newSpanContextImpl(parent SpanContext) *spanContextImpl {
	sc := &spanContextImpl{
		traceFlags: FlagsSampled,
	}

	// Generate new span ID from buffered CSPRNG (amortises syscall overhead)
	rng.read(sc.spanID[:])

	// Inherit or generate trace ID
	if parent != nil && parent.IsValid() {
		// Parse parent trace ID
		sc.traceID = parseTraceID(parent.TraceID())
		sc.remote = false
	} else {
		// Generate new trace ID
		rng.read(sc.traceID[:])
	}

	return sc
}

// NewRemoteSpanContext creates a SpanContext from remote context data.
// Used by propagators to create contexts from incoming requests.
func NewRemoteSpanContext(traceID [16]byte, spanID [8]byte, flags TraceFlags) SpanContext {
	return &spanContextImpl{
		traceID:    traceID,
		spanID:     spanID,
		traceFlags: flags,
		remote:     true,
	}
}

// TraceID implements SpanContext.
func (sc *spanContextImpl) TraceID() string {
	return hex.EncodeToString(sc.traceID[:])
}

// SpanID implements SpanContext.
func (sc *spanContextImpl) SpanID() string {
	return hex.EncodeToString(sc.spanID[:])
}

// TraceFlags implements SpanContext.
func (sc *spanContextImpl) TraceFlags() TraceFlags {
	return sc.traceFlags
}

// IsValid implements SpanContext.
func (sc *spanContextImpl) IsValid() bool {
	return sc != nil && sc.traceID != [16]byte{} && sc.spanID != [8]byte{}
}

// IsRemote implements SpanContext.
func (sc *spanContextImpl) IsRemote() bool {
	return sc.remote
}

// IsSampled implements SpanContext.
func (sc *spanContextImpl) IsSampled() bool {
	return sc.traceFlags.IsSampled()
}

// TraceIDBytes returns the trace ID as bytes.
func (sc *spanContextImpl) TraceIDBytes() [16]byte {
	return sc.traceID
}

// SpanIDBytes returns the span ID as bytes.
func (sc *spanContextImpl) SpanIDBytes() [8]byte {
	return sc.spanID
}

// Ensure spanContextImpl implements SpanContext.
var _ SpanContext = (*spanContextImpl)(nil)
