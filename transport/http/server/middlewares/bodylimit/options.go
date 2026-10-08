// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package bodylimit

//go:generate go run github.com/altessa-s/go-atlas/cmd/optgen generate --type=options

import (
	"net/http"

	"github.com/altessa-s/go-atlas/transport/http/server/responder"
)

// LimitFunc returns the body limit for one request: a positive value is the
// limit in bytes, 0 applies the static limit passed to [New], and a negative
// value lifts the limit for the request.
type LimitFunc func(r *http.Request) int64

// options configures the body limit middleware.
type options struct {
	// requireContentLength rejects body-bearing requests
	// (POST/PUT/PATCH/DELETE) that arrive without a declared Content-Length
	// (e.g. Transfer-Encoding: chunked) with 411 Length Required. Off by
	// default — chunked uploads are still accepted and the size cap is
	// enforced lazily by http.MaxBytesReader once the handler reads the
	// body. Turn this on when you need the cap rejected UP FRONT (e.g. a
	// public endpoint where a chunked-drip attacker should not be allowed
	// to keep a request goroutine alive until the full ReadTimeout).
	requireContentLength bool
	// limitFunc computes the limit per request (an upload route, an
	// authenticated user) — see [LimitFunc]. Nil applies the static limit.
	limitFunc LimitFunc
	// errorWriter writes the 413 / 411 responses, e.g. an HTML error page.
	// Nil uses the [responder.ErrorWriter] from the request context, with a
	// plain-text fallback (see [responder.WriteError]).
	errorWriter responder.ErrorWriter
}
