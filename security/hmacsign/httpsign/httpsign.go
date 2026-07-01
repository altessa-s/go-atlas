// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package httpsign

import (
	"bytes"
	"fmt"
	"io"
	"net/http"

	"github.com/altessa-s/go-atlas/security/hmacsign"
)

// Verify reads r's body (capped at [WithMaxBytes]), verifies the signature in
// v's scheme header against it, and restores r.Body so a handler can read it
// again. It returns the verified raw body, or an error: [ErrBodyTooLarge], or a
// verification failure forwarded from [hmacsign.Verifier.Verify]
// ([hmacsign.ErrSignatureMismatch], [hmacsign.ErrMalformedSignature],
// [hmacsign.ErrTimestampOutOfTolerance]).
func Verify(v *hmacsign.Verifier, r *http.Request, opts ...Option) ([]byte, error) {
	o := newOptions(opts...)
	body, err := readBody(r, o.maxBytes)
	if err != nil {
		return nil, err
	}
	if err := v.Verify(r.Header.Get(v.HeaderName()), body); err != nil {
		return nil, err
	}
	return body, nil
}

// Middleware returns a wrapper that verifies the signature of every request
// before the next handler runs. On failure it writes the response with the
// configured [ErrorHandler] (default: 413 for an oversized body, 401 otherwise)
// and does not call next. On success the verified body is left readable on
// r.Body.
func Middleware(v *hmacsign.Verifier, opts ...Option) func(http.Handler) http.Handler {
	o := newOptions(opts...)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, err := readBody(r, o.maxBytes)
			if err == nil {
				err = v.Verify(r.Header.Get(v.HeaderName()), body)
			}
			if err != nil {
				o.errorHandler(w, r, err)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// readBody reads at most maxBytes from r.Body and restores it as a fresh reader
// so the caller (and any downstream handler) can read the same bytes again. A
// body larger than maxBytes yields [ErrBodyTooLarge].
func readBody(r *http.Request, maxBytes int64) ([]byte, error) {
	if r.Body == nil {
		return nil, nil
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBytes+1))
	_ = r.Body.Close()
	if err != nil {
		return nil, fmt.Errorf("httpsign: read body: %w", err)
	}
	if int64(len(body)) > maxBytes {
		return nil, ErrBodyTooLarge
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	return body, nil
}
