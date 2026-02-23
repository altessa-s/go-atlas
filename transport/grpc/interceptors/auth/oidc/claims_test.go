// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oidc

import "testing"

func TestClaims_Fields(t *testing.T) {
	c := Claims{
		Subject:           "sub1",
		PreferredUsername: "user1",
		Email:             "a@b.com",
		Issuer:            "https://issuer",
		Audience:          []string{"aud1"},
		Scopes:            []string{"openid", "profile"},
	}
	if c.Subject != "sub1" {
		t.Fatalf("Subject = %q", c.Subject)
	}
	if c.Email != "a@b.com" {
		t.Fatalf("Email = %q", c.Email)
	}
	if len(c.Scopes) != 2 {
		t.Fatalf("Scopes len = %d", len(c.Scopes))
	}
}
