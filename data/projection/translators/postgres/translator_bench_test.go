// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package postgres_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/data/projection"
	"github.com/altessa-s/go-atlas/data/projection/translators/postgres"
)

func BenchmarkTranslate(b *testing.B) {
	tr, err := postgres.NewTranslator(
		projection.WithAllowedFields("name", "email", "createTime"),
		projection.WithFieldMapping(map[string]string{"createTime": "created_at"}),
		projection.WithRequiredFields("id"),
	)
	if err != nil {
		b.Fatal(err)
	}
	spec := projection.Spec{Paths: []string{"createTime", "email", "name"}}
	for b.Loop() {
		_, _ = tr.Translate(spec)
	}
}
