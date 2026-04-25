// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package tokenbucket

import "context"

type clientIPContextKey struct{}

type authTokenContextKey struct{}

// ContextWithClientIP returns a new context that carries the client IP address.
// The transport layer should call this before invoking Limit() so that
// ExtractClientIp can retrieve the IP without transport-specific dependencies.
func ContextWithClientIP(ctx context.Context, ip string) context.Context {
	return context.WithValue(ctx, clientIPContextKey{}, ip)
}

// ContextWithAuthToken returns a new context that carries the authentication token.
// The transport layer should call this before invoking Limit() so that
// ExtractAuthToken can retrieve the token without transport-specific dependencies.
func ContextWithAuthToken(ctx context.Context, token string) context.Context {
	return context.WithValue(ctx, authTokenContextKey{}, token)
}

func clientIPFromContext(ctx context.Context) string {
	ip, ok := ctx.Value(clientIPContextKey{}).(string)
	if !ok {
		return ""
	}
	return ip
}

func authTokenFromContext(ctx context.Context) string {
	token, ok := ctx.Value(authTokenContextKey{}).(string)
	if !ok {
		return ""
	}
	return token
}
