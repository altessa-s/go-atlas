// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package fieldpolicy_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/data/internal/fieldpolicy"
)

func BenchmarkAllowListAllows(b *testing.B) {
	list := fieldpolicy.NewAllowList("createdAt", "slug", "address.*", "user.profile.*")
	for b.Loop() {
		_ = list.Allows("user.profile.name")
	}
}

func BenchmarkMappingApply(b *testing.B) {
	m := fieldpolicy.Mapping{}.
		WithExact(map[string]string{"createdAt": "created_at"}).
		WithPrefixes(map[string]string{"address.": "addr.", "user.profile.": "up."})
	for b.Loop() {
		_ = m.Apply("user.profile.name")
	}
}

func BenchmarkOverlaps(b *testing.B) {
	for b.Loop() {
		_ = fieldpolicy.Overlaps("credentials.password", "credentials")
	}
}
