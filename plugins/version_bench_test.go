// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package plugins

import "testing"

func BenchmarkMajorOf(b *testing.B) {
	cases := []string{
		"",
		"1",
		"v1.2.3",
		"v2.0.0-rc.1",
		"v0.0.0-20240101000000-abcdef",
		"3.0.0+build.5",
	}
	for b.Loop() {
		for _, v := range cases {
			_ = majorOf(v)
		}
	}
}

func BenchmarkManager_checkHostMajor(b *testing.B) {
	mgr := NewManager(
		WithHostVersion("2.5.3"),
		WithHostVersionMode(HostVersionEnforce),
	)
	b.Cleanup(func() { _ = mgr.Close() })

	desc := &Descriptor{Name: "p", HostVersion: "2.1.0"}
	for b.Loop() {
		_ = mgr.checkHostMajor(desc)
	}
}
