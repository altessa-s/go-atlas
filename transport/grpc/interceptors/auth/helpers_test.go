// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package auth

import (
	"testing"

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
				if err == nil {
					t.Fatal("expected error")
				}
				st, ok := status.FromError(err)
				if !ok || st.Code() != codes.Unauthenticated {
					t.Fatalf("expected Unauthenticated, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tok != tt.wantTok {
				t.Fatalf("token = %q, want %q", tok, tt.wantTok)
			}
		})
	}
}

func TestExtractTokenFromHeader_Custom(t *testing.T) {
	extractor := ExtractTokenFromHeader("x-api-key", func(t string) (string, error) {
		return t, nil
	})

	ctx := grpcmetadata.NewIncomingContext(t.Context(), grpcmetadata.Pairs("x-api-key", "my-key"))
	tok, err := extractor.ExtractToken(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if tok != "my-key" {
		t.Fatalf("token = %q", tok)
	}
}

func BenchmarkExtractBearerToken(b *testing.B) {
	extractor := ExtractBearerToken()
	ctx := grpcmetadata.NewIncomingContext(b.Context(), grpcmetadata.Pairs("authorization", "Bearer tok"))
	for b.Loop() {
		extractor.ExtractToken(ctx) //nolint:errcheck
	}
}
