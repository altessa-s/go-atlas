// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package masking_test

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/altessa-s/go-atlas/observability/slog/handler/masking"
)

func ExampleURLMask() {
	mask := masking.URLMask()

	// Mask S3 bucket names in URLs
	fmt.Println(mask("https://s3.amazonaws.com/my-secret-bucket/path/to/file.mp3"))

	// Mask cloud storage container names
	fmt.Println(mask("https://storage.yandexcloud.net/audio-files/operations/123/audio.wav"))

	// Preserves query parameters and fragments
	fmt.Println(mask("https://example.com/bucket/dir/file.txt?param=value#section"))

	// Output:
	// https://s3.amazonaws.com/my-***/file.mp3
	// https://storage.yandexcloud.net/aud***/audio.wav
	// https://example.com/buc***/file.txt?param=value#section
}

func ExampleS3URLMask() {
	mask := masking.S3URLMask()

	// Extract and show operation IDs
	fmt.Println(mask("https://s3.region.amazonaws.com/bucket/operations/op-123/file.mp3"))

	// Handle UUID-format operation IDs
	fmt.Println(mask("https://s3.example.com/bucket/jobs/123e4567-e89b-12d3-a456-426614174000/result.csv"))

	// Regular URL without operation ID
	fmt.Println(mask("https://storage.endpoint.com/bucket/some/path/file.mp3"))

	// Output:
	// s3://.../<op-123>/file.mp3
	// s3://.../<123e4567-e89b-12d3-a456-426614174000>/result.csv
	// s3://.../file.mp3
}

func ExampleURLMask_withLogger() {
	// Create a masking handler that automatically masks URLs
	handler := masking.NewHandler(
		slog.NewJSONHandler(os.Stdout, nil),
		masking.WithField("url", masking.URLMask()),
		masking.WithField("s3_url", masking.S3URLMask()),
	)

	logger := slog.New(handler)

	// URLs will be automatically masked in logs
	logger.Info("request processed",
		slog.String("url", "https://s3.amazonaws.com/my-secret-bucket/data/file.mp3"),
		slog.String("s3_url", "https://s3.region.amazonaws.com/bucket/operations/op-456/result.json"),
	)
}
