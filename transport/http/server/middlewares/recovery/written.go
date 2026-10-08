// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package recovery

import (
	"bufio"
	"errors"
	"net"
	"net/http"
)

// trackingWriter records whether the response has started: a status line
// sent by WriteHeader, a body Write (which implies 200), or a Flush.
type trackingWriter struct {
	http.ResponseWriter
	started bool
}

func (w *trackingWriter) WriteHeader(code int) {
	// Marked only once the underlying writer accepted the code: an invalid
	// code panics there and sends nothing.
	w.ResponseWriter.WriteHeader(code)
	// 1xx informational responses do not start the final response.
	if code >= http.StatusOK || code == http.StatusSwitchingProtocols {
		w.started = true
	}
}

func (w *trackingWriter) Write(b []byte) (int, error) {
	w.started = true
	return w.ResponseWriter.Write(b) //nolint:gosec // Transparent proxy forwarding handler output
}

// Flush implements [http.Flusher]; see FlushError.
func (w *trackingWriter) Flush() {
	_ = w.FlushError() //nolint:errcheck // http.Flusher has no error result
}

// FlushError flushes the underlying writer through [http.ResponseController],
// which prefers it to Flush, so its error — [http.ErrNotSupported] for a
// writer that cannot flush — reaches the caller. A flush starts the response
// unless it is unsupported: a failing one may already have sent the headers.
func (w *trackingWriter) FlushError() error {
	err := http.NewResponseController(w.ResponseWriter).Flush()
	if !errors.Is(err, http.ErrNotSupported) {
		w.started = true
	}
	return err
}

// Hijack implements [http.Hijacker] for handlers that assert it (WebSocket
// upgrades); a hijacked connection counts as started. It fails with
// [http.ErrNotSupported] when the underlying writer cannot hijack.
func (w *trackingWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	conn, rw, err := http.NewResponseController(w.ResponseWriter).Hijack()
	if err == nil {
		w.started = true
	}
	return conn, rw, err
}

// Unwrap returns the underlying writer for [http.ResponseController].
func (w *trackingWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

// HeadersWritten reports whether the response behind w — the writer the
// middleware passes to a [PanicHandler] — has already started: its status
// and headers are on the wire, so the handler can no longer send an error
// page and should only log, or abort the connection by panicking with
// [http.ErrAbortHandler]. It reports false for a writer the middleware did
// not wrap.
func HeadersWritten(w http.ResponseWriter) bool {
	for w != nil {
		if tw, ok := w.(*trackingWriter); ok {
			return tw.started
		}
		u, ok := w.(interface{ Unwrap() http.ResponseWriter })
		if !ok {
			return false
		}
		w = u.Unwrap()
	}
	return false
}
