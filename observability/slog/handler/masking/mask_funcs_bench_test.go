// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package masking_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/observability/slog/handler/masking"
)

func BenchmarkPartialMask(b *testing.B) {
	mask := masking.PartialMask(2, 2, "*")
	input := "sensitive-data-12345"

	for b.Loop() {
		_ = mask(input)
	}
}

func BenchmarkEmailMask(b *testing.B) {
	mask := masking.EmailMask()
	input := "user@example.com"

	for b.Loop() {
		_ = mask(input)
	}
}

func BenchmarkPhoneMask(b *testing.B) {
	mask := masking.PhoneMask()
	input := "+1-234-567-8900"

	for b.Loop() {
		_ = mask(input)
	}
}

func BenchmarkCreditCardMask(b *testing.B) {
	mask := masking.CreditCardMask()
	input := "4111111111111111"

	for b.Loop() {
		_ = mask(input)
	}
}

// maskBenchCase names a single sub-benchmark input for runMaskBench.
type maskBenchCase struct {
	name  string
	input string
}

// runMaskBench runs one isolated sub-benchmark per case, each measuring only
// the mask invocation on that case's input.
func runMaskBench(b *testing.B, mask masking.MaskFunc, cases []maskBenchCase) {
	b.Helper()
	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			input := tc.input
			for b.Loop() {
				_ = mask(input)
			}
		})
	}
}

func BenchmarkURLMask(b *testing.B) {
	runMaskBench(b, masking.URLMask(), []maskBenchCase{
		{"typical_s3", "https://s3.endpoint.com/my-bucket/path/to/file.mp3"},
		{"long_path", "https://storage.yandexcloud.net/audio-files/operations/123/audio.wav"},
		{"with_query", "https://example.com/bucket/dir/file.txt?param=value"},
		{"single_file", "https://cdn.example.com/image.png"},
	})
}

func BenchmarkS3URLMask(b *testing.B) {
	runMaskBench(b, masking.S3URLMask(), []maskBenchCase{
		{"with_op_id", "https://s3.region.amazonaws.com/bucket/operations/op-123/file.mp3"},
		{"with_operation", "https://storage.endpoint.com/bucket/operation-abc/data.json"},
		{"with_uuid", "https://s3.example.com/bucket/jobs/123e4567-e89b-12d3-a456-426614174000/result.csv"},
		{"without_op_id", "https://storage.endpoint.com/bucket/some/path/file.mp3"},
	})
}

func BenchmarkCachedPartialMask(b *testing.B) {
	mask := masking.CachedPartialMask(2, 2, "*")
	input := "sensitive-data-12345"

	for b.Loop() {
		_ = mask(input)
	}
}
