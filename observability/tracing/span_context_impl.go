// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package tracing

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
)

// randBufSize is the size of each buffered CSPRNG chunk. Each refill is a
// single crypto/rand read; between refills the bytes are served from memory.
const randBufSize = 4096

// randBuf is a chunk of crypto/rand output owned by one goroutine at a time.
type randBuf struct {
	buf [randBufSize]byte
	pos int
}

// randBufs hands out per-P chunks so concurrent span starts do not serialize
// on one lock. A chunk is checked out exclusively and every byte is served at
// most once; chunks dropped by the GC only discard unread bytes.
var randBufs = sync.Pool{
	// pos starts at randBufSize to force a fill on first use.
	New: func() any { return &randBuf{pos: randBufSize} },
}

// readRandom fills a and b with cryptographically secure random bytes from a
// single chunk checkout; b may be nil. A root span needs both IDs, so one
// pool round trip serves them.
func readRandom(a, b []byte) {
	r := randBufs.Get().(*randBuf) //nolint:errcheck,forcetypeassert // only *randBuf is stored
	n := len(a) + len(b)
	if r.pos+n > randBufSize {
		_, _ = rand.Read(r.buf[:]) //nolint:errcheck // only fails on reader exhaustion
		r.pos = 0
	}
	r.pos += copy(a, r.buf[r.pos:])
	r.pos += copy(b, r.buf[r.pos:])
	randBufs.Put(r)
}

// spanContextImpl is the concrete implementation of SpanContext.
type spanContextImpl struct {
	traceID    [16]byte
	spanID     [8]byte
	traceFlags TraceFlags
	remote     bool
	traceState string
}

// newSpanContextImpl creates a new SpanContext.
// If parent is valid, the trace ID is inherited.
func newSpanContextImpl(parent SpanContext) *spanContextImpl {
	sc := &spanContextImpl{
		traceFlags: FlagsSampled,
	}

	// IDs come from a buffered CSPRNG (amortizes syscall overhead).
	if parent != nil && parent.IsValid() {
		readRandom(sc.spanID[:], nil)
		// Inherit parent trace ID
		sc.traceID = traceIDOf(parent)
		sc.remote = false
		if state, ok := parent.(interface{ TraceState() string }); ok {
			sc.traceState = state.TraceState()
		}
	} else {
		readRandom(sc.spanID[:], sc.traceID[:])
	}

	return sc
}

// TraceState returns the inherited or sampler-updated vendor trace state.
func (sc *spanContextImpl) TraceState() string { return sc.traceState }

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

// rawSpanContext exposes binary IDs so in-package consumers skip the hex
// round trip of TraceID/SpanID.
type rawSpanContext interface {
	TraceIDBytes() [16]byte
	SpanIDBytes() [8]byte
}

// traceIDOf returns sc's trace ID, parsing the hex form only for foreign
// SpanContext implementations.
func traceIDOf(sc SpanContext) [16]byte {
	if raw, ok := sc.(rawSpanContext); ok {
		return raw.TraceIDBytes()
	}
	return parseTraceID(sc.TraceID())
}

// spanIDOf returns sc's span ID, parsing the hex form only for foreign
// SpanContext implementations.
func spanIDOf(sc SpanContext) [8]byte {
	if raw, ok := sc.(rawSpanContext); ok {
		return raw.SpanIDBytes()
	}
	return parseSpanID(sc.SpanID())
}

// Ensure spanContextImpl implements SpanContext.
var _ SpanContext = (*spanContextImpl)(nil)
