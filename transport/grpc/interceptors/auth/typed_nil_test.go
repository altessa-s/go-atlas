// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package auth_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/auth"
)

func TestTokenCredentialsTypedNil(t *testing.T) {
	t.Parallel()
	req := auth.Request{AuthMethod: auth.MethodToken, Payload: (*auth.TokenCredentials)(nil)}
	got, ok := req.TokenCredentials()
	require.False(t, ok)
	require.Nil(t, got)
}

func TestTokenCredentialsNilRequest(t *testing.T) {
	t.Parallel()
	var req *auth.Request
	got, ok := req.TokenCredentials()
	require.False(t, ok)
	require.Nil(t, got)
}
