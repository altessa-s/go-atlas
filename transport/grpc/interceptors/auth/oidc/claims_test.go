// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oidc

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestClaims_Fields(t *testing.T) {
	c := Claims{
		Subject:           "sub1",
		PreferredUsername: "user1",
		Email:             "a@b.com",
		Issuer:            "https://issuer",
		Audience:          []string{"aud1"},
		Scopes:            []string{"openid", "profile"},
	}
	require.Equal(t, "sub1", c.Subject)
	require.Equal(t, "a@b.com", c.Email)
	require.Len(t, c.Scopes, 2)
}
