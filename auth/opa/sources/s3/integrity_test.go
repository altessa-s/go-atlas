// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package s3_test

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	corehash "github.com/altessa-s/go-atlas-core/encoding/hash"
	s3source "github.com/altessa-s/go-atlas/auth/opa/sources/s3"
)

// Verification stays optional, but a source built without a manifest says so
// once, at construction.
func TestNew_WarnsWithoutChecksums(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		opts      []s3source.Option
		wantWarns int
	}{
		{name: "no manifest", wantWarns: 1},
		{name: "manifest", opts: []s3source.Option{s3source.WithChecksums(map[string]string{"main.rego": strings.Repeat("0", 64)})}},
		{name: "empty manifest rejects every file", opts: []s3source.Option{s3source.WithChecksums(map[string]string{})}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var logs bytes.Buffer
			base := []s3source.Option{
				s3source.WithS3Client(&mockS3Client{}),
				s3source.WithBucket("test-bucket"),
				s3source.WithLogger(slog.New(slog.NewTextHandler(&logs, nil))),
			}
			source, err := s3source.New(append(base, tc.opts...)...)
			require.NoError(t, err)
			require.NoError(t, source.Close())
			require.Equal(t, tc.wantWarns, strings.Count(logs.String(), "level=WARN"))
		})
	}
}

func TestSource_Fetch_VerifiesChecksums(t *testing.T) {
	t.Parallel()

	const policy = "package main\ndefault allow = false"
	digest := corehash.SHA256HexBytes([]byte(policy))

	tests := []struct {
		name      string
		checksums map[string]string
		wantErr   error
	}{
		{name: "match", checksums: map[string]string{"main.rego": digest}},
		{name: "mismatch", checksums: map[string]string{"main.rego": strings.Repeat("0", 64)}, wantErr: s3source.ErrChecksumMismatch},
		{name: "unexpected file", checksums: map[string]string{}, wantErr: s3source.ErrUnexpectedPolicyFile},
		{name: "missing file", checksums: map[string]string{"main.rego": digest, "other.rego": digest}, wantErr: s3source.ErrMissingPolicyFile},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			client := &mockS3Client{objects: map[string][]byte{"policies/main.rego": []byte(policy)}}
			source, err := s3source.New(
				s3source.WithS3Client(client),
				s3source.WithBucket("test-bucket"),
				s3source.WithPrefix("policies/"),
				s3source.WithChecksums(tc.checksums),
			)
			require.NoError(t, err)
			t.Cleanup(func() { _ = source.Close() })

			bundle, err := source.Fetch(t.Context())
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, policy, string(bundle.Modules["main.rego"]))
		})
	}
}
