// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package tokenbucket

//go:generate go run github.com/altessa-s/go-atlas/tools/codegen/optgen generate

import (
	"context"
	"log/slog"
)

const (
	// DefaultCacheSize is the default size for the IP cache.
	DefaultCacheSize = 1000
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
	iPCacheSize            int              `optgen:"default=DefaultCacheSize"`
	extractToken           ExtractTokenFunc `optgen:"default=defaultExtractAuthToken"`
	extractClientIPAddress ExtractIPFunc    `optgen:"default=defaultExtractClientIp"`
	clientService          ClientService
	logger                 *slog.Logger
}

// ExtractClientIp extracts the client IP address from the context bridge.
// The transport layer must call ContextWithClientIP before invoking Limit()
// so that this function can retrieve the IP.
// Returns empty string if no IP is found in the context.
func ExtractClientIp(ctx context.Context) string {
	return clientIPFromContext(ctx)
}

// ExtractAuthToken extracts the authentication token from the context bridge.
// The transport layer must call ContextWithAuthToken before invoking Limit()
// so that this function can retrieve the token.
// Returns empty string if no token is found in the context.
func ExtractAuthToken(ctx context.Context) string {
	return authTokenFromContext(ctx)
}
