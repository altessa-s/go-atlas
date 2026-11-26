// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package respwriter

import (
	"bufio"
	"bytes"
	"errors"
	"net"
	"net/http"
	"sync"
)

// maxPooledCapacity is the largest body buffer capacity a released
// [Recorder] keeps for reuse; a larger buffer is dropped.
const maxPooledCapacity = 64 << 10

// StructuredMarker is implemented by a response writer wrapper that treats a
// response differently once the writer package declares it structured.
type StructuredMarker interface {
	// MarkStructured tells the wrapper that the response about to be written
	// is complete and already formatted for the client.
	MarkStructured()
}

// MarkStructured calls [StructuredMarker.MarkStructured] on every writer on
// the Unwrap chain of w that implements it. The writer package calls it
// before writing the header of a response it formatted itself.
func MarkStructured(w http.ResponseWriter) {
	for w != nil {
		if m, ok := w.(StructuredMarker); ok {
			m.MarkStructured()
		}
		u, ok := w.(interface{ Unwrap() http.ResponseWriter })
		if !ok {
			return
		}
		w = u.Unwrap()
	}
}

// IsFinal reports whether code is a final status: a response other than an
// informational 1xx one, or 101 Switching Protocols.
func IsFinal(code int) bool {
	return code >= http.StatusOK || code == http.StatusSwitchingProtocols
}

// Recorder wraps [http.ResponseWriter] and records the final status, the
// body size and optionally a bounded prefix of the body. Obtain one with
// [NewRecorder] and return it with [Recorder.Release].
//
// A Recorder is NOT safe for concurrent use.
type Recorder struct {
	http.ResponseWriter
	status    int
	committed bool
	capture   bool
	limit     int
	body      bytes.Buffer
	// written counts the body bytes accepted by the underlying writer,
	// captured or not.
	written  int64
	writeErr error
	hijacked bool
}

var recorderPool = sync.Pool{New: func() any { return &Recorder{} }}

// NewRecorder returns a pooled [Recorder] over w. With capture it keeps up to
// limit bytes of the body.
func NewRecorder(w http.ResponseWriter, capture bool, limit int) *Recorder {
	r := recorderPool.Get().(*Recorder) //nolint:errcheck // the pool only holds *Recorder
	r.Reset(w, capture, limit)
	return r
}

// Release returns r to the pool; r must come from [NewRecorder].
func (r *Recorder) Release() {
	r.Clear()
	recorderPool.Put(r)
}

// Reset prepares r, which may be a zero value, to record a response written
// to w. With capture it keeps up to limit bytes of the body.
func (r *Recorder) Reset(w http.ResponseWriter, capture bool, limit int) {
	r.ResponseWriter = w
	r.status = http.StatusOK
	r.committed = false
	r.capture = capture
	r.limit = limit
	r.body.Reset()
	r.written = 0
	r.writeErr = nil
	r.hijacked = false
}

// Clear drops the references r holds. A body buffer that grew past
// maxPooledCapacity is dropped.
func (r *Recorder) Clear() {
	r.ResponseWriter = nil
	r.writeErr = nil
	if r.body.Cap() > maxPooledCapacity {
		r.body = bytes.Buffer{}
	} else {
		r.body.Reset()
	}
}

// WriteHeader forwards the status and records it when it is final. The first
// final status wins; informational statuses other than 101 are forwarded and
// not recorded.
func (r *Recorder) WriteHeader(code int) {
	if r.committed {
		return
	}
	r.ResponseWriter.WriteHeader(code)
	if IsFinal(code) {
		r.status = code
		r.committed = true
	}
}

// Write forwards b and counts and captures the bytes the underlying writer
// accepted, including a partial write that comes with an error.
func (r *Recorder) Write(b []byte) (int, error) {
	// nosemgrep: go.lang.security.audit.xss.no-direct-write-to-responsewriter
	n, err := r.ResponseWriter.Write(b) //nolint:gosec // Transparent proxy forwarding handler output
	if !r.committed {
		r.status = http.StatusOK
		r.committed = true
	}
	if r.writeErr == nil {
		r.writeErr = err
	}
	r.written += int64(n)

	if r.capture {
		if remaining := r.limit - r.body.Len(); remaining > 0 {
			r.body.Write(b[:min(n, remaining)])
		}
	}
	return n, err
}

// Flush implements [http.Flusher]; see [Recorder.FlushError].
func (r *Recorder) Flush() { _ = r.FlushError() } //nolint:errcheck // http.Flusher has no error result

// FlushError flushes the underlying writer. A flush that succeeds sends the
// header the server would send, so the record commits to 200 when no final
// status was written yet. A flush that fails for another reason than the
// writer not supporting it may already have sent that header, so the record
// commits as well, and the failure is kept as the first write error: a
// response whose delivery failed is not complete.
func (r *Recorder) FlushError() error {
	err := http.NewResponseController(r.ResponseWriter).Flush()
	if err == nil || !errors.Is(err, http.ErrNotSupported) {
		if !r.committed {
			r.status = http.StatusOK
			r.committed = true
		}
		if err != nil && r.writeErr == nil {
			r.writeErr = err
		}
	}
	return err
}

// Hijack implements [http.Hijacker] by delegating to the underlying writer.
// A successful hijack hands the connection off: the response counts as
// committed with status 101 and [Recorder.Hijacked] reports true.
func (r *Recorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	conn, rw, err := http.NewResponseController(r.ResponseWriter).Hijack()
	if err == nil {
		r.hijacked = true
		if !r.committed {
			r.status = http.StatusSwitchingProtocols
			r.committed = true
		}
	}
	return conn, rw, err
}

// Hijacked reports whether the connection was handed off.
func (r *Recorder) Hijacked() bool { return r.hijacked }

// Push implements [http.Pusher] when the underlying writer does.
func (r *Recorder) Push(target string, opts *http.PushOptions) error {
	if p, ok := r.ResponseWriter.(http.Pusher); ok {
		return p.Push(target, opts)
	}
	return http.ErrNotSupported
}

// Unwrap returns the underlying writer.
func (r *Recorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }

// StatusCode returns the recorded final status, 200 when none was written.
func (r *Recorder) StatusCode() int { return r.status }

// HeadersWritten reports whether a final status reached the client.
func (r *Recorder) HeadersWritten() bool { return r.committed }

// BytesWritten returns the body bytes written through r, including those
// beyond the capture limit.
func (r *Recorder) BytesWritten() int64 { return r.written }

// WriteError returns the first body write error, nil when every write
// succeeded.
func (r *Recorder) WriteError() error { return r.writeErr }

// Body returns the captured body prefix.
func (r *Recorder) Body() []byte { return r.body.Bytes() }

// BodyString returns the captured body prefix as a string.
func (r *Recorder) BodyString() string { return r.body.String() }
