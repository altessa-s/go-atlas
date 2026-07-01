// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oauth2client

import "testing"

// BenchmarkExchangerFormValues measures building the RFC 8693 request body, the
// per-exchange hot path.
func BenchmarkExchangerFormValues(b *testing.B) {
	e := NewExchanger("https://idp.example/token", "svc", "s3cr3t")
	req := ExchangeRequest{
		SubjectToken: "inbound-access-token",
		Audience:     "https://downstream.internal",
		Resource:     "https://api.internal/v1",
		Scopes:       []string{"orders:read", "orders:write"},
	}
	b.ReportAllocs()
	for b.Loop() {
		_ = e.formValues(req)
	}
}

// BenchmarkParseTokenResponse measures decoding a token-exchange success body.
func BenchmarkParseTokenResponse(b *testing.B) {
	body := []byte(`{"access_token":"abc","issued_token_type":"` +
		TokenTypeAccessToken + `","token_type":"Bearer","expires_in":3600,"scope":"a b c"}`)
	b.ReportAllocs()
	for b.Loop() {
		if _, err := parseTokenResponse(body); err != nil {
			b.Fatal(err)
		}
	}
}
