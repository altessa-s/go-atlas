// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package auth

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas-net/httpx"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	grpcmetadata "google.golang.org/grpc/metadata"
)

func TestExtractBearerToken(t *testing.T) {
	t.Parallel()
	extractor := ExtractBearerToken()
	invalidToken := status.Error(codes.Unauthenticated, "missing or invalid bearer token")

	cases := []struct {
		name    string
		headers []string
		want    string
		wantErr error
	}{
		{"valid", []string{"Bearer abc.def=="}, "abc.def==", nil},
		{"lowercase", []string{"bearer xyz"}, "xyz", nil},
		{"missing", nil, "", status.Error(codes.Unauthenticated, "missing or invalid token")},
		{"empty", []string{""}, "", invalidToken},
		{"wrong scheme", []string{"Basic abc"}, "", invalidToken},
		{"two spaces", []string{"Bearer  abc"}, "", invalidToken},
		{"invalid character", []string{"Bearer a:b"}, "", invalidToken},
		{"duplicate", []string{"Bearer abc", "Bearer abc"}, "", invalidToken},
		{"too long", []string{"Bearer " + strings.Repeat("a", httpx.MaxBearerLength+1)}, "", invalidToken},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := grpcmetadata.NewIncomingContext(t.Context(), grpcmetadata.MD{"authorization": tc.headers})
			token, err := extractor.ExtractToken(ctx)
			require.ErrorIs(t, err, tc.wantErr)
			require.Equal(t, tc.want, token)
		})
	}
}

func TestExtractTokenFromHeader_Custom(t *testing.T) {
	extractor := ExtractTokenFromHeader("x-api-key", func(t string) (string, error) {
		return t, nil
	})

	ctx := grpcmetadata.NewIncomingContext(t.Context(), grpcmetadata.Pairs("x-api-key", "my-key"))
	tok, err := extractor.ExtractToken(ctx)
	require.NoError(t, err)
	require.Equal(t, "my-key", tok)
}

func BenchmarkExtractBearerToken(b *testing.B) {
	extractor := ExtractBearerToken()
	ctx := grpcmetadata.NewIncomingContext(b.Context(), grpcmetadata.Pairs("authorization", "Bearer tok"))
	for b.Loop() {
		extractor.ExtractToken(ctx) //nolint:errcheck
	}
}
