// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package endpointfilter

import "testing"

func FuzzChecker_ShouldFilter(f *testing.F) {
	f.Add("/health")
	f.Add("/api/v1/users")
	f.Add("")
	f.Add("/HEALTH")

	c, err := New([]string{"/health", "/ready", "/metrics"})
	if err != nil {
		f.Fatalf("New() error = %v", err)
	}

	f.Fuzz(func(t *testing.T, path string) {
		// Should not panic
		c.ShouldFilter(path)
	})
}
