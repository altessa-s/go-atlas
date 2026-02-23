// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package prometheus

import "testing"

func FuzzBuildMetricName(f *testing.F) {
	f.Add("myapp", "http", "requests_total")
	f.Add("", "", "")
	f.Add("ns", "", "suffix")

	f.Fuzz(func(t *testing.T, namespace, subsystem, suffix string) {
		result := BuildMetricName(namespace, subsystem, suffix)
		if namespace == "" && subsystem == "" && suffix == "" && result != "" {
			t.Fatal("expected empty result for all empty inputs")
		}
	})
}
