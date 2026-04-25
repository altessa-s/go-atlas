// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package tlss3_test

import (
	"context"
	"testing"

	tlss3 "github.com/altessa-s/go-atlas/security/tlsutils/providers/s3"
)

func FuzzNew_InvalidInputs(f *testing.F) {
	f.Add("", "", "", "")
	f.Add("bucket", "", "", "")
	f.Add("", "cert.crt", "", "")
	f.Add("bucket", "cert.crt", "key.key", "")
	f.Add("bucket", "cert.crt", "key.key", "password")

	f.Fuzz(func(t *testing.T, bucket, certKey, privKeyKey, password string) {
		// Should fail gracefully without panic (no S3 client provided)
		p, err := tlss3.New(bucket, certKey, privKeyKey, password)
		if err == nil && p != nil {
			_ = p.Close(context.Background())
		}
	})
}
