// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package shared

import "testing"

func FuzzBuildMetricName(f *testing.F) {
	f.Add("app", "http", "requests")
	f.Add("", "", "requests")
	f.Add("app", "", "")

	f.Fuzz(func(t *testing.T, service, sub, name string) {
		// Should not panic
		result := BuildMetricName(service, sub, name)
		_ = result
	})
}

func FuzzJoinScope(f *testing.F) {
	f.Add("parent", "child")
	f.Add("", "child")
	f.Add("parent", "")

	f.Fuzz(func(t *testing.T, parent, child string) {
		// Should not panic
		result := JoinScope(parent, child, '_')
		if parent == "" && child != "" && result != child {
			t.Errorf("expected %q, got %q", child, result)
		}
		if child == "" && parent != "" && result != parent {
			t.Errorf("expected %q, got %q", parent, result)
		}
	})
}
