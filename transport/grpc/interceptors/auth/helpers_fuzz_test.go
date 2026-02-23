// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package auth

import (
	"testing"

	grpcmetadata "google.golang.org/grpc/metadata"
)

func FuzzExtractBearerToken(f *testing.F) {
	f.Add("Bearer valid-token")
	f.Add("bearer lowercase")
	f.Add("Basic abc")
	f.Add("")
	f.Add("BearerNoSpace")

	extractor := ExtractBearerToken()

	f.Fuzz(func(t *testing.T, auth string) {
		ctx := grpcmetadata.NewIncomingContext(t.Context(), grpcmetadata.Pairs("authorization", auth))
		extractor.ExtractToken(ctx) //nolint:errcheck
	})
}
