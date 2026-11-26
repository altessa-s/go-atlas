// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package auth

//go:generate go tool optgen generate --type=clientOptions --option-type=ClientOption --output=client_options_gen.go

// Defaults of the client token credentials.
const (
	// DefaultClientHeaderName is the metadata header that carries the token.
	DefaultClientHeaderName = "authorization"

	// DefaultClientScheme is the authentication scheme prefix of the token.
	DefaultClientScheme = "Bearer"
)

// clientOptions configures the credentials made by [NewPerRPCCredentials]. All
// fields are populated by generated [ClientOption] setters.
type clientOptions struct {
	// headerName is the metadata header that carries the token. gRPC metadata
	// keys are lowercase, so the name is trimmed and lowercased.
	headerName string `optgen:"default=DefaultClientHeaderName" optval:"lower"`

	// scheme is the authentication scheme prefix; use noScheme to send the bare token.
	scheme string `optgen:"default=DefaultClientScheme"`

	// noScheme sends the bare token without a scheme prefix; it overrides scheme.
	noScheme bool
}
