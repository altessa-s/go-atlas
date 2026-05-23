// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package masking_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/observability/slog/handler/masking"
)

func TestPartialMask(t *testing.T) {
	mask := masking.PartialMask(2, 2, "*")
	tests := []struct {
		input string
		want  string
	}{
		{"abcdef", "ab**ef"},
		{"ab", "**"},
		{"a", "*"},
		{"", ""},
	}
	for _, tt := range tests {
		got := mask(tt.input)
		require.Equal(t, tt.want, got, "PartialMask(%q)", tt.input)
	}
}

func TestSmartMask(t *testing.T) {
	mask := masking.SmartMask()
	got := mask("secret123")
	require.Contains(t, got, "se")
	require.Contains(t, got, "23")
}

func TestFullMask(t *testing.T) {
	mask := masking.FullMask()
	got := mask("anything")
	require.Equal(t, "********", got)
}

func TestFixedMask(t *testing.T) {
	mask := masking.FixedMask("[REDACTED]")
	got := mask("secret")
	require.Equal(t, "[REDACTED]", got)
}

func TestEmailMask(t *testing.T) {
	mask := masking.EmailMask()
	got := mask("user@example.com")
	require.Contains(t, got, "@example.com")
	require.False(t, strings.HasPrefix(got, "user@"), "EmailMask should mask local part, got %q", got)
}

func TestPhoneMask(t *testing.T) {
	mask := masking.PhoneMask()
	got := mask("+1234567890")
	require.NotEqual(t, "+1234567890", got, "PhoneMask should mask the number")
}

func TestCreditCardMask(t *testing.T) {
	mask := masking.CreditCardMask()
	got := mask("4111111111111111")
	require.NotEqual(t, "4111111111111111", got, "CreditCardMask should mask the number")
}

func TestHashMask(t *testing.T) {
	mask := masking.HashMask("hash:")
	got := mask("secret")
	require.True(t, strings.HasPrefix(got, "hash:"), "HashMask() = %q, want prefix hash:", got)
}

func TestPatternMask(t *testing.T) {
	mask := masking.PatternMask(`\d+`, masking.FixedMask("***"))
	got := mask("order-12345-abc")
	require.NotContains(t, got, "12345", "PatternMask should mask digits")
}

func TestCachedPartialMask(t *testing.T) {
	mask := masking.CachedPartialMask(2, 2, "*")
	got := mask("abcdefgh")
	require.True(t, strings.HasPrefix(got, "ab"), "CachedPartialMask(abcdefgh) = %q", got)
	require.True(t, strings.HasSuffix(got, "gh"), "CachedPartialMask(abcdefgh) = %q", got)
}

func TestPrecomputedMasks(t *testing.T) {
	pm := masking.NewPrecomputedMasks("*")
	got := pm.GetMask(3)
	require.Equal(t, "***", got)
	got = pm.GetMask(10)
	require.Len(t, got, 10)
}

func TestURLMask(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
		{
			name:  "not a URL",
			input: "just-a-filename.txt",
			want:  "jus***txt",
		},
		{
			name:  "S3 URL with bucket and path",
			input: "https://s3.endpoint.com/my-bucket/path/to/file.mp3",
			want:  "https://s3.endpoint.com/my-***/file.mp3",
		},
		{
			name:  "Yandex storage URL",
			input: "https://storage.yandexcloud.net/audio-files/operations/123/audio.wav",
			want:  "https://storage.yandexcloud.net/aud***/audio.wav",
		},
		{
			name:  "URL with query string",
			input: "https://example.com/bucket/dir/file.txt?param=value",
			want:  "https://example.com/buc***/file.txt?param=value",
		},
		{
			name:  "URL with fragment",
			input: "https://example.com/container/path/doc.pdf#page=5",
			want:  "https://example.com/con***/doc.pdf#page=5",
		},
		{
			name:  "URL with single path component",
			input: "https://cdn.example.com/image.png",
			want:  "https://cdn.example.com/ima***",
		},
		{
			name:  "URL without path",
			input: "https://example.com",
			want:  "https://example.com",
		},
		{
			name:  "Short bucket name",
			input: "https://s3.amazonaws.com/ab/file.txt",
			want:  "https://s3.amazonaws.com/**/file.txt",
		},
	}

	mask := masking.URLMask()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := mask(tt.input)
			require.Equal(t, tt.want, got, "URLMask(%q)", tt.input)
		})
	}
}

func TestS3URLMask(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
		{
			name:  "not a URL",
			input: "just-a-filename.txt",
			want:  "s3://.../just-a-filename.txt",
		},
		{
			name:  "S3 URL with operation ID (op- prefix)",
			input: "https://s3.region.amazonaws.com/bucket/operations/op-123/file.mp3",
			want:  "s3://.../<op-123>/file.mp3",
		},
		{
			name:  "URL with operation- prefix",
			input: "https://storage.endpoint.com/bucket/operation-abc/data.json",
			want:  "s3://.../<operation-abc>/data.json",
		},
		{
			name:  "URL with UUID format operation ID",
			input: "https://s3.example.com/bucket/jobs/123e4567-e89b-12d3-a456-426614174000/result.csv",
			want:  "s3://.../<123e4567-e89b-12d3-a456-426614174000>/result.csv",
		},
		{
			name:  "URL without operation ID",
			input: "https://storage.endpoint.com/bucket/some/path/file.mp3",
			want:  "s3://.../file.mp3",
		},
		{
			name:  "URL with query parameters",
			input: "https://s3.amazonaws.com/bucket/file.pdf?versionId=abc123",
			want:  "s3://.../file.pdf",
		},
		{
			name:  "URL without path",
			input: "https://s3.amazonaws.com",
			want:  "s3://***",
		},
		{
			name:  "URL with only bucket",
			input: "https://s3.amazonaws.com/mybucket/",
			want:  "s3://.../",
		},
		{
			name:  "Local file path",
			input: "/var/data/uploads/file.dat",
			want:  "s3://.../file.dat",
		},
	}

	mask := masking.S3URLMask()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := mask(tt.input)
			require.Equal(t, tt.want, got, "S3URLMask(%q)", tt.input)
		})
	}
}
