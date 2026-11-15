// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package auth

import (
	"context"
	"net/netip"
	"time"

	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/metadata"
)

// Method represents the authentication method used.
type Method string

const (
	// MethodToken represents token authentication.
	MethodToken Method = "token"
)

// Request represents an authentication request containing the base authentication
// metadata and the authentication payload. The Payload field contains method-specific
// credentials (e.g., TokenCredentials for token-based auth).
type Request struct {
	Base
	Payload any
}

// TokenCredentials extracts and returns TokenCredentials from the request payload.
// It returns the credentials and true if the request uses token authentication
// and the payload is valid TokenCredentials, otherwise returns nil and false.
func (r *Request) TokenCredentials() (*TokenCredentials, bool) {
	if r == nil || !r.IsToken() {
		return nil, false
	}
	tc, ok := r.Payload.(*TokenCredentials)
	return tc, ok && tc != nil
}

// UserAgentFromCallMeta creates a UserAgent instance from gRPC call metadata.
// It extracts the client's IP address and user agent string from the call metadata.
func UserAgentFromCallMeta(meta *metadata.CallMetadata) *UserAgent {
	return &UserAgent{
		RemoteAddr: meta.ClientPerIP,
		UserAgent:  meta.ClientUserAgent,
	}
}

// Credentials represents the complete authentication credentials after successful
// authentication. It contains the base authentication metadata, custom auth data
// returned by the auth function, and all request headers.
type Credentials struct {
	Base
	// Data contains custom authentication data returned by the auth function
	// (e.g., user ID, roles, permissions, JWT claims).
	Data any
	// Headers contains the gRPC metadata headers from the incoming request,
	// minus authorization, cookie and proxy-authorization. The server
	// interceptor fills it only when a [ClientAuth] is configured; handlers
	// read the request metadata directly otherwise.
	Headers map[string]string
}

// Base contains the common authentication metadata shared between Request and Credentials.
// It includes information about the authentication method, timing, client details,
// and the gRPC method being called.
type Base struct {
	// AuthMethod specifies which authentication method was used (e.g., MethodToken).
	AuthMethod Method
	// AuthenticatedAt is the timestamp when authentication was performed.
	AuthenticatedAt time.Time
	// UserAgent contains client identification information (IP address, user agent string).
	UserAgent *UserAgent
	// ServiceName is the gRPC service name (e.g., "myservice.v1.MyService").
	ServiceName string
	// MethodName is the gRPC method name without service (e.g., "GetUser").
	MethodName string
	// FullyMethodName is the complete gRPC method path (e.g., "/myservice.v1.MyService/GetUser").
	FullyMethodName string
}

// IsAuthenticated returns true if credentials are present and valid.
func (c *Base) IsAuthenticated() bool {
	if c == nil {
		return false
	}
	return c.AuthMethod != ""
}

// IsToken returns true if authentication was done using a token.
func (c *Base) IsToken() bool {
	return c != nil && c.AuthMethod == MethodToken
}

// UserAgent represents client identification information including the remote
// address and user agent string extracted from the gRPC request metadata.
type UserAgent struct {
	// RemoteAddr is the IP address of the client making the request.
	RemoteAddr netip.Addr

	// UserAgent is the user agent string provided by the client (e.g., "grpc-go/1.50.0").
	UserAgent string
}

// credentialsKey is the context key under which the server interceptor stores
// the authenticated [Credentials].
type credentialsKey struct{}

// CredentialsFromContext returns the [Credentials] the server interceptor
// stored after a successful authentication. The bool is false when the call
// was not authenticated by the interceptor (for example an ignored method).
func CredentialsFromContext(ctx context.Context) (Credentials, bool) {
	c, ok := ctx.Value(credentialsKey{}).(Credentials)
	return c, ok
}
