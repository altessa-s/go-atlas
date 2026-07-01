// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oauth2client

import (
	"strings"

	"github.com/altessa-s/go-atlas/auth/principal"

	"golang.org/x/oauth2"
)

// Principal builds a canonical [github.com/altessa-s/go-atlas/auth/principal.Principal]
// describing the identity a fetched token represents: subject is the
// caller-supplied service identity (typically the OAuth2 client_id) and scopes
// are those the IdP granted, read from the token's "scope" field
// (space-delimited, per RFC 6749 §5.1).
//
// It does NOT verify tok — verifying inbound tokens is
// [github.com/altessa-s/go-atlas/auth/oidc]'s job. This is a convenience for
// representing this service's own outbound identity with the same Principal the
// scope enforcer consumes, so an acquired token and an enforced one share a
// type. A nil token yields a Principal carrying only the subject.
func Principal(subject string, tok *oauth2.Token) principal.Principal {
	p := principal.Principal{Subject: subject}
	if tok == nil {
		return p
	}
	if scope, ok := tok.Extra("scope").(string); ok && scope != "" {
		p.Scopes = strings.Fields(scope)
	}
	return p
}
