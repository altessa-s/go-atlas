// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package httpsign is the net/http adapter for
// [github.com/altessa-s/go-atlas/security/hmacsign]. It reads the raw request
// body once, verifies the provider's signature header against it, and restores
// the body so a downstream handler can read it again.
//
// The core hmacsign package is transport-free; this adapter is the thin seam
// that binds it to *http.Request, kept in a separate package so the primitive
// never imports net/http.
//
// # Middleware
//
//	v := hmacsign.NewVerifier(hmacsign.Stripe(), secret)
//	mux.Handle("/webhooks/stripe", httpsign.Middleware(v)(handler))
//
// A request with a missing, malformed, or unauthentic signature is rejected
// before it reaches the handler; the handler runs only for authentic bodies and
// reads them from r.Body as usual.
//
// # Handler helper
//
//	body, err := httpsign.Verify(v, r)
//	if err != nil {
//	    http.Error(w, "unauthorized", http.StatusUnauthorized)
//	    return
//	}
//
// [Verify] returns the verified raw body for handlers that prefer an explicit
// call to wrapping middleware. Both cap the body at [WithMaxBytes] (default one
// mebibyte) so an oversized request cannot exhaust memory before verification.
package httpsign
