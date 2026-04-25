// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package tlss3_test

import (
	"testing"
	"time"

	tlss3 "github.com/altessa-s/go-atlas/security/tlsutils/providers/s3"
)

func BenchmarkNew(b *testing.B) {
	client, _, _ := newMockClient(b)

	b.ResetTimer()
	for b.Loop() {
		p, err := tlss3.New("my-bucket", "certs/server.crt", "certs/server.key", "",
			tlss3.WithS3Client(client),
			tlss3.WithPollInterval(time.Hour),
		)
		if err != nil {
			b.Fatal(err)
		}
		p.Close(b.Context())
	}
}

func BenchmarkTLSConfig(b *testing.B) {
	client, _, _ := newMockClient(b)

	p, err := tlss3.New("my-bucket", "certs/server.crt", "certs/server.key", "",
		tlss3.WithS3Client(client),
		tlss3.WithPollInterval(time.Hour),
	)
	if err != nil {
		b.Fatal(err)
	}
	defer p.Close(b.Context())

	b.ResetTimer()
	for b.Loop() {
		_, _ = p.TLSConfig()
	}
}

func BenchmarkType(b *testing.B) {
	client, _, _ := newMockClient(b)

	p, err := tlss3.New("my-bucket", "certs/server.crt", "certs/server.key", "",
		tlss3.WithS3Client(client),
		tlss3.WithPollInterval(time.Hour),
	)
	if err != nil {
		b.Fatal(err)
	}
	defer p.Close(b.Context())

	b.ResetTimer()
	for b.Loop() {
		_ = p.Type()
	}
}
