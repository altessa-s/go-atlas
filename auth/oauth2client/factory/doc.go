// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package factory builds client-side OAuth2 token acquisition from a
// [github.com/altessa-s/go-atlas/config.OAuth2Client] template and injected
// dependencies.
//
// [Builder.Build] returns a self-refreshing client_credentials
// [golang.org/x/oauth2.TokenSource] — the machine-to-machine case; the token
// endpoint comes either from config (tokenUrl) or, when the config sets
// discoveryUrl, from an OIDC discovery source injected via
// [Builder.UseTokenEndpointSource] (satisfied by
// [github.com/altessa-s/go-atlas/auth/oidc.Provider]). [Builder.BuildExchanger]
// returns an RFC 8693 [github.com/altessa-s/go-atlas/auth/oauth2client.Exchanger]
// from the same configuration.
//
// Usage:
//
//	src, err := factory.New(cfg.OAuth2Client).
//	    UseTokenEndpointSource(oidcProvider). // only needed when cfg uses discoveryUrl
//	    Build(ctx)
package factory
