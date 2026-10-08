// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package clientip

import (
	"log/slog"
	"net/netip"
)

// Constants for default client IP extractor options.
const (
	// DefaultTrustedProxiesCount is the default number of trusted proxies in X-Forwarded-For chain.
	DefaultTrustedProxiesCount = 0

	// DefaultHeadersEnabled indicates whether headers are processed by default.
	DefaultHeadersEnabled = true

	// DefaultCacheSize is the default IP address cache size.
	DefaultCacheSize = 1000

	// DefaultCacheDisabled indicates whether cache is disabled by default.
	DefaultCacheDisabled = false
)

//go:generate go run github.com/altessa-s/go-atlas/cmd/optgen generate --type=options

// options holds [Extractor] configuration. Fields are populated by
// functional [Option] values and have sensible defaults set via optgen.
type options struct {
	trustedPeers        []netip.Prefix
	trustedProxies      []netip.Prefix
	trustedProxiesCount uint     `optgen:"default=DefaultTrustedProxiesCount"`
	headers             []string `optgen:"default=DefaultHeaders()"`
	headersEnabled      bool     `optgen:"default=DefaultHeadersEnabled"`
	logger              *slog.Logger
	cacheSize           int  `optgen:"default=DefaultCacheSize"`
	cacheDisabled       bool `optgen:"default=DefaultCacheDisabled"`
}
