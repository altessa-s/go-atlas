// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package auth

import (
	"net/netip"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

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
			got := tt.base.IsAuthenticated()
			require.Equal(t, tt.want, got)
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
			got := tt.base.IsToken()
			require.Equal(t, tt.want, got)
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
			require.Equal(t, tt.wantOK, ok)
			require.False(t, ok && tc.Token.Expose() != tt.wantTok)
		})
	}
}

func TestUserAgentFromCallMeta(t *testing.T) {
	meta := &metadata.CallMetadata{
		ClientPerIP:     netip.MustParseAddr("10.0.0.1"),
		ClientUserAgent: "test-agent",
	}
	ua := UserAgentFromCallMeta(meta)
	require.Equal(t, meta.ClientPerIP, ua.RemoteAddr)
	require.Equal(t, "test-agent", ua.UserAgent)
}

func TestMethodToken(t *testing.T) {
	require.EqualValues(t, "token", MethodToken)
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
	require.True(t, c.IsAuthenticated(), "should be authenticated")
	require.True(t, c.IsToken(), "should be token")
}
