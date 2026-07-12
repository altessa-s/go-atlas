// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package ipacl

import (
	"net/netip"
	"regexp"
	"testing"
)

func newBenchRegistry() *Registry {
	reg := NewRegistry(PolicyDeny)
	reg.Register("/admin.AdminService/Delete", &AccessRule{
		Allowlist: []netip.Prefix{prefix("10.1.1.0/24"), prefix("192.168.0.0/16")},
		Denylist:  []netip.Prefix{prefix("172.16.0.0/12")},
	})
	reg.Register("/api.Service/Get", &AccessRule{
		Allowlist: []netip.Prefix{prefix("10.0.0.0/8")},
	})
	reg.RegisterPattern(regexp.MustCompile(`^/internal\.`), &AccessRule{
		Allowlist: []netip.Prefix{prefix("10.0.0.0/8")},
	})
	return reg
}

func BenchmarkRegistry_Evaluate_ExactMatch(b *testing.B) {
	reg := newBenchRegistry()
	ip := addr("10.1.1.5")

	b.ReportAllocs()
	for b.Loop() {
		reg.Evaluate(ip, "/admin.AdminService/Delete")
	}
}

func BenchmarkRegistry_Evaluate_PatternMatch(b *testing.B) {
	reg := newBenchRegistry()
	ip := addr("10.2.3.4")

	b.ReportAllocs()
	for b.Loop() {
		reg.Evaluate(ip, "/internal.Debug/Dump")
	}
}
