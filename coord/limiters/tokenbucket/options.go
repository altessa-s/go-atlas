// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package tokenbucket

//go:generate go tool optgen generate

import (
	"context"
	"log/slog"

	"github.com/altessa-s/go-atlas/observability/metrics"
)

const (
	// DefaultCacheSize is the default size for the IP cache.
	DefaultCacheSize = 1000

	// DefaultIPv6KeyPrefixBits is the default IPv6 prefix length that shares
	// one rate-limit bucket. A /64 is the smallest subnet commonly assigned to
	// one subscriber, who can rotate through all of its addresses at will.
	DefaultIPv6KeyPrefixBits = 64
)

// ExtractTokenFunc defines a function that extracts authentication tokens from context.
// It receives the request context, returning the token string
// or empty string if no token is found.
type ExtractTokenFunc func(ctx context.Context) string

// ExtractIPFunc defines a function that extracts client IP addresses from context.
// It should return the most accurate client IP available, considering proxy headers
// and load balancer forwarding.
type ExtractIPFunc func(ctx context.Context) string

func defaultExtractClientIp(_ context.Context) string { return "" }

func defaultExtractAuthToken(_ context.Context) string { return "" }

// options contains RuleLimiter configuration.
type options struct {
	iPCacheSize int `optgen:"default=DefaultCacheSize"`
	// iPv6KeyPrefixBits is the IPv6 prefix length (0-128) whose addresses
	// share one storage key; 128 keys every address separately. Rules still
	// match the full client address.
	iPv6KeyPrefixBits      int              `optgen:"default=DefaultIPv6KeyPrefixBits"`
	extractToken           ExtractTokenFunc `optgen:"default=defaultExtractAuthToken"`
	extractClientIPAddress ExtractIPFunc    `optgen:"default=defaultExtractClientIp"`
	clientService          ClientService
	// rules are IP and CIDR specific limits that override the default
	// settings. They are matched most specific first: exact IP addresses,
	// then CIDR subnets from the longest prefix down.
	rules  []*RateLimitRule
	logger *slog.Logger
	meter  metrics.Meter `optgen:"notnil"`
}
