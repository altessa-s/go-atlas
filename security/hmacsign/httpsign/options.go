// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package httpsign

//go:generate go run github.com/altessa-s/go-atlas/cmd/optgen generate

import (
	"errors"
	"net/http"
)

// DefaultMaxBytes is the default cap on the request body read for verification:
// one mebibyte. Webhook payloads are small; the cap stops an oversized request
// from exhausting memory before its signature is even checked.
const DefaultMaxBytes int64 = 1 << 20

// ErrorHandler writes the response for a request whose signature could not be
// verified. err is the failure ([hmacsign.ErrSignatureMismatch],
// [hmacsign.ErrMalformedSignature], [hmacsign.ErrTimestampOutOfTolerance], or
// [ErrBodyTooLarge]).
type ErrorHandler func(w http.ResponseWriter, r *http.Request, err error)

// defaultErrorHandler maps an oversized body to 413 and every other failure to
// 401, writing a plain-text reason (never echoing the error detail to the
// client).
var defaultErrorHandler ErrorHandler = func(w http.ResponseWriter, _ *http.Request, err error) {
	if errors.Is(err, ErrBodyTooLarge) {
		http.Error(w, "request entity too large", http.StatusRequestEntityTooLarge)
		return
	}
	http.Error(w, "invalid signature", http.StatusUnauthorized)
}

// options carries the adapter tunables.
type options struct {
	maxBytes     int64        `optgen:"default=DefaultMaxBytes"`
	errorHandler ErrorHandler `optgen:"default=defaultErrorHandler"`
}
