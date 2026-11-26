// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package client_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/transport/grpc/client"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/auth"

	"golang.org/x/oauth2"

	"google.golang.org/grpc"
)

// TestTokenOverPlaintextIsRefused configures a token on a plaintext client:
// the call must fail and the server must never see the token.
func TestTokenOverPlaintextIsRefused(t *testing.T) {
	t.Parallel()

	b := startBackend(t)
	c, err := client.New(t.Context(), "passthrough:///bufnet",
		client.WithInsecure(), client.WithoutHealthCheck(),
		client.WithDialOptions(b.dialer(), grpc.WithPerRPCCredentials(auth.NewPerRPCCredentials(auth.StaticTokenProvider("secret")))),
	)
	if err == nil {
		t.Cleanup(func() { _ = c.Close(context.WithoutCancel(t.Context())) })
		_, err = invoke(t, c, "hi")
	}
	require.Error(t, err)
	require.Empty(t, b.lastAuthorization(), "the token reached a plaintext server")
}

// TestInsecureTokenCredentialsAreTheExplicitOptIn pins the named way to send a
// token in cleartext.
func TestInsecureTokenCredentialsAreTheExplicitOptIn(t *testing.T) {
	t.Parallel()

	b := startBackend(t)
	creds := client.NewInsecureTokenCredentials(oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "dev-token"}))
	c := newClient(t, b, client.WithDialOptions(grpc.WithPerRPCCredentials(creds)))

	_, err := invoke(t, c, "hi")
	require.NoError(t, err)
	require.Equal(t, "Bearer dev-token", b.lastAuthorization())
}
