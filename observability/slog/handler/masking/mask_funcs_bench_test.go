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

func BenchmarkURLMask(b *testing.B) {
	mask := masking.URLMask()
	inputs := []string{
		"https://s3.endpoint.com/my-bucket/path/to/file.mp3",
		"https://storage.yandexcloud.net/audio-files/operations/123/audio.wav",
		"https://example.com/bucket/dir/file.txt?param=value",
		"https://cdn.example.com/image.png",
	}

	b.Run("typical_s3", func(b *testing.B) {
		input := inputs[0]
		for b.Loop() {
			_ = mask(input)
		}
	})

	b.Run("long_path", func(b *testing.B) {
		input := inputs[1]
		for b.Loop() {
			_ = mask(input)
		}
	})

	b.Run("with_query", func(b *testing.B) {
		input := inputs[2]
		for b.Loop() {
			_ = mask(input)
		}
	})

	b.Run("single_file", func(b *testing.B) {
		input := inputs[3]
		for b.Loop() {
			_ = mask(input)
		}
	})
}

func BenchmarkS3URLMask(b *testing.B) {
	mask := masking.S3URLMask()
	inputs := []string{
		"https://s3.region.amazonaws.com/bucket/operations/op-123/file.mp3",
		"https://storage.endpoint.com/bucket/operation-abc/data.json",
		"https://s3.example.com/bucket/jobs/123e4567-e89b-12d3-a456-426614174000/result.csv",
		"https://storage.endpoint.com/bucket/some/path/file.mp3",
	}

	b.Run("with_op_id", func(b *testing.B) {
		input := inputs[0]
		for b.Loop() {
			_ = mask(input)
		}
	})

	b.Run("with_operation", func(b *testing.B) {
		input := inputs[1]
		for b.Loop() {
			_ = mask(input)
		}
	})

	b.Run("with_uuid", func(b *testing.B) {
		input := inputs[2]
		for b.Loop() {
			_ = mask(input)
		}
	})

	b.Run("without_op_id", func(b *testing.B) {
		input := inputs[3]
		for b.Loop() {
			_ = mask(input)
		}
	})
}

func BenchmarkCachedPartialMask(b *testing.B) {
	mask := masking.CachedPartialMask(2, 2, "*")
	input := "sensitive-data-12345"

	for b.Loop() {
		_ = mask(input)
	}
}
