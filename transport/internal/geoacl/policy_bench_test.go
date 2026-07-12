// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package geoacl

import (
	"regexp"
	"testing"
)

func newBenchRegistry() *Registry {
	reg := NewRegistry(PolicyDeny)
	reg.Register("/admin.AdminService/Delete", &AccessRule{
		DenyCountries:   []string{"RU", "CN"},
		AllowCountries:  []string{"US", "CA"},
		AllowContinents: []string{"EU"},
	})
	reg.RegisterPattern(regexp.MustCompile(`^/internal\.`), &AccessRule{
		AllowCountries: []string{"US"},
	})
	return reg
}

func BenchmarkRegistry_Evaluate_ExactMatch(b *testing.B) {
	reg := newBenchRegistry()
	resolver := &mockResolver{geo: GeoInfo{ContinentCode: "NA", CountryCode: "US", RegionCode: "CA"}}
	ctx := b.Context()

	b.ReportAllocs()
	for b.Loop() {
		if _, err := reg.Evaluate(ctx, resolver, testIP, "/admin.AdminService/Delete"); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRegistry_Evaluate_PatternMatch(b *testing.B) {
	reg := newBenchRegistry()
	resolver := &mockResolver{geo: GeoInfo{ContinentCode: "NA", CountryCode: "US"}}
	ctx := b.Context()

	b.ReportAllocs()
	for b.Loop() {
		if _, err := reg.Evaluate(ctx, resolver, testIP, "/internal.Debug/Dump"); err != nil {
			b.Fatal(err)
		}
	}
}
