// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package limiters

import "context"

type clientIPContextKey struct{}

type authTokenContextKey struct{}

// ContextWithClientIP returns a new context that carries the client IP address.
func ContextWithClientIP(ctx context.Context, ip string) context.Context {
	return context.WithValue(ctx, clientIPContextKey{}, ip)
}

// ContextWithAuthToken returns a new context that carries the authentication token.
func ContextWithAuthToken(ctx context.Context, token string) context.Context {
	return context.WithValue(ctx, authTokenContextKey{}, token)
}

// ClientIPFromContext returns the client IP address carried by ctx, or an empty string.
func ClientIPFromContext(ctx context.Context) string {
	ip, _ := ctx.Value(clientIPContextKey{}).(string)
	return ip
}

// AuthTokenFromContext returns the authentication token carried by ctx, or an empty string.
func AuthTokenFromContext(ctx context.Context) string {
	token, _ := ctx.Value(authTokenContextKey{}).(string)
	return token
}
