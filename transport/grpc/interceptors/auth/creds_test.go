// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package auth

import (
	"net/netip"
	"testing"
	"time"

	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/metadata"
)

func TestBase_IsAuthenticated(t *testing.T) {
	tests := []struct {
		name string
		base *Base
		want bool
	}{
		{"nil", nil, false},
		{"empty", &Base{}, false},
		{"token", &Base{AuthMethod: MethodToken}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.base.IsAuthenticated(); got != tt.want {
				t.Fatalf("IsAuthenticated() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBase_IsToken(t *testing.T) {
	tests := []struct {
		name string
		base *Base
		want bool
	}{
		{"nil", nil, false},
		{"empty", &Base{}, false},
		{"token", &Base{AuthMethod: MethodToken}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.base.IsToken(); got != tt.want {
				t.Fatalf("IsToken() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRequest_TokenCredentials(t *testing.T) {
	tests := []struct {
		name    string
		req     Request
		wantOK  bool
		wantTok string
	}{
		{
			name:   "not_token_method",
			req:    Request{Base: Base{AuthMethod: "other"}, Payload: &TokenCredentials{Token: "t"}},
			wantOK: false,
		},
		{
			name:   "nil_payload",
			req:    Request{Base: Base{AuthMethod: MethodToken}, Payload: nil},
			wantOK: false,
		},
		{
			name:    "valid",
			req:     Request{Base: Base{AuthMethod: MethodToken}, Payload: &TokenCredentials{Token: "abc"}},
			wantOK:  true,
			wantTok: "abc",
		},
		{
			name:   "wrong_payload_type",
			req:    Request{Base: Base{AuthMethod: MethodToken}, Payload: "not-token-creds"},
			wantOK: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tc, ok := tt.req.TokenCredentials()
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if ok && tc.Token != tt.wantTok {
				t.Fatalf("Token = %q, want %q", tc.Token, tt.wantTok)
			}
		})
	}
}

func TestUserAgentFromCallMeta(t *testing.T) {
	meta := &metadata.CallMetadata{
		ClientPerIP:     netip.MustParseAddr("10.0.0.1"),
		ClientUserAgent: "test-agent",
	}
	ua := UserAgentFromCallMeta(meta)
	if ua.RemoteAddr != meta.ClientPerIP {
		t.Fatalf("RemoteAddr = %v", ua.RemoteAddr)
	}
	if ua.UserAgent != "test-agent" {
		t.Fatalf("UserAgent = %q", ua.UserAgent)
	}
}

func TestMethodToken(t *testing.T) {
	if MethodToken != "token" {
		t.Fatalf("MethodToken = %q", MethodToken)
	}
}

func TestCredentials(t *testing.T) {
	c := Credentials{
		Base: Base{
			AuthMethod:      MethodToken,
			AuthenticatedAt: time.Now(),
		},
		Data:    "user-123",
		Headers: map[string]string{"authorization": "Bearer tok"},
	}
	if !c.IsAuthenticated() {
		t.Fatal("should be authenticated")
	}
	if !c.IsToken() {
		t.Fatal("should be token")
	}
}
