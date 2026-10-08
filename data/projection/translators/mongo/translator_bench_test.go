// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/data/projection"
)

func BenchmarkTranslate(b *testing.B) {
	tr := newTranslator(b,
		projection.WithAllowedFields("name", "email", "address.*", "createTime"),
		projection.WithFieldMapping(map[string]string{"createTime": "created_at"}),
		projection.WithRequiredFields("_id", "version"),
	)
	spec := projection.Spec{Paths: []string{"address", "address.city", "createTime", "email", "name"}}
	for b.Loop() {
		_, _ = tr.Translate(spec)
	}
}

func BenchmarkTranslateDefault(b *testing.B) {
	tr := newTranslator(b, projection.WithDeniedFields("password"))
	for b.Loop() {
		_, _ = tr.Translate(projection.Spec{})
	}
}
