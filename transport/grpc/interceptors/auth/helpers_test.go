// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package auth

import (
	"testing"

	"github.com/stretchr/testify/require"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	grpcmetadata "google.golang.org/grpc/metadata"
)

func TestExtractBearerToken(t *testing.T) {
	extractor := ExtractBearerToken()

	tests := []struct {
		name    string
		md      grpcmetadata.MD
		wantTok string
		wantErr bool
	}{
		{
			name:    "valid_bearer",
			md:      grpcmetadata.Pairs("authorization", "Bearer my-token"),
			wantTok: "my-token",
		},
		{
			name:    "case_insensitive",
			md:      grpcmetadata.Pairs("authorization", "bearer my-token"),
			wantTok: "my-token",
		},
		{
			name:    "missing_header",
			md:      grpcmetadata.MD{},
			wantErr: true,
		},
		{
			name:    "wrong_scheme",
			md:      grpcmetadata.Pairs("authorization", "Basic abc"),
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := grpcmetadata.NewIncomingContext(t.Context(), tt.md)
			tok, err := extractor.ExtractToken(ctx)
			if tt.wantErr {
				require.NotNil(t, err, "expected error")
				st, ok := status.FromError(err)
				require.True(t, ok, "expected Unauthenticated, got %v", err)
				require.Equal(t, codes.Unauthenticated, st.Code())
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.wantTok, tok)
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
