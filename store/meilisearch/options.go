// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package meilisearch

//go:generate go tool optgen generate

import (
	"log/slog"
	"net/http"
	"time"
)

// DefaultTimeout applies when [WithTimeout] is not used or set to a
// non-positive value. Bounds every outbound Meilisearch request (search,
// index, delete, health) including the synchronous health probe in [New].
const DefaultTimeout = 30 * time.Second

// options holds the configuration applied by generated [Option] values.
// The client constructor gives [WithHTTPClient] precedence over [WithTimeout].
type options struct {
	apiKey  string        `opt:"APIKey"`
	timeout time.Duration `optval:"positive" optgen:"default=DefaultTimeout"`
	logger  *slog.Logger  `optgen:"default=slog.New(slog.DiscardHandler)"`
	// WithHTTPClient injects a fully configured *http.Client into the
	// Meilisearch SDK. When set, the client's Timeout takes precedence over
	// [WithTimeout] (the latter is ignored because the operator passing a
	// custom client has already expressed intent about timeout semantics).
	//
	// Typical use case: the infrastructure factory builds an *http.Client
	// with a *tls.Config derived from tlsconfig.Client, so TLS settings
	// (CA pool, mTLS client cert, MinVersion, SkipVerifyMode) reach the SDK
	// without leaking into this package's option surface.
	//
	// A nil client is a no-op (the timeout-derived default is preserved).
	httpClient *http.Client `optgen:"notnil"`
}
