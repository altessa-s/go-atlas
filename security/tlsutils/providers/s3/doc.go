// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package tlss3 provides an S3-based TLS certificate provider with automatic
// polling for changes. Supports AWS S3 and S3-compatible storage (e.g. MinIO)
// with SSE-S3, SSE-KMS, and SSE-C server-side encryption.
//
// Example:
//
//	provider, _ := tlss3.New("my-bucket", "certs/server.crt", "certs/server.key", "",
//	    tlss3.WithS3Client(s3Client),
//	    tlss3.WithPollInterval(5 * time.Minute),
//	    tlss3.WithLogger(slog.Default()),
//	)
//	defer provider.Close(ctx)
//	config, _ := provider.TLSConfig()
package tlss3
