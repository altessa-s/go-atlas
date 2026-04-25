// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package tlsfile_test

import (
	"testing"

	tlsfile "github.com/altessa-s/go-atlas/security/tlsutils/providers/file"
)

func FuzzNewWithCertAndKey_InvalidInputs(f *testing.F) {
	f.Add("", "", "")
	f.Add("cert.pem", "", "")
	f.Add("", "key.pem", "")
	f.Add("/nonexistent/cert.pem", "/nonexistent/key.pem", "")
	f.Add("cert.pem", "key.pem", "password")

	f.Fuzz(func(t *testing.T, certFile, keyFile, password string) {
		// Most inputs should fail gracefully without panic
		f, err := tlsfile.NewWithCertAndKey(certFile, keyFile, password)
		if err == nil && f != nil {
			// If it somehow succeeds (unlikely with fuzz), clean up
			_ = f.Close(nil)
		}
	})
}
