// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package endpointrule_test

import (
	"regexp"
	"testing"

	"github.com/altessa-s/go-atlas/transport/internal/endpointrule"
)

func newBenchRegistry() *endpointrule.Registry[rule] {
	r := &endpointrule.Registry[rule]{}
	r.Register("/svc.Exact", &rule{})
	r.RegisterPattern(regexp.MustCompile(`^/admin\.`), &rule{})
	r.SetDefault(&rule{})
	return r
}

func BenchmarkRegistry_Lookup(b *testing.B) {
	r := newBenchRegistry()

	for _, bc := range []struct{ name, endpoint string }{
		{"exact", "/svc.Exact"},
		{"pattern", "/admin.Users"},
		{"default", "/other.Method"},
	} {
		b.Run(bc.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				_, _ = r.Lookup(bc.endpoint)
			}
		})
	}
}
