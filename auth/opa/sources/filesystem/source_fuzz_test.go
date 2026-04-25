// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package filesystem_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/auth/opa/sources/filesystem"
)

func FuzzNew(f *testing.F) {
	// Seed corpus with various path patterns
	f.Add("/tmp/policies")
	f.Add("/tmp/policy.rego")
	f.Add("")
	f.Add(".")
	f.Add("..")
	f.Add("/")

	f.Fuzz(func(t *testing.T, path string) {
		// New should not panic with any input
		// It may return an error for invalid paths, which is expected
		source, err := filesystem.New(path)
		if err == nil && source != nil {
			// If New succeeds, source should be closeable
			_ = source.Close()
		}
	})
}
