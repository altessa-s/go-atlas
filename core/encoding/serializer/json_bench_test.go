// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package serializer_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/core/encoding/serializer"
	"github.com/altessa-s/go-atlas/internal/testhelpers"
)

func BenchmarkJSON(b *testing.B) {
	s := &serializer.JSON{}
	val := testhelpers.Person{Name: "Antonio", Age: 30}

	b.Run("Serialize", func(b *testing.B) {
		for b.Loop() {
			_, _ = s.Serialize(val)
		}
	})

	b.Run("Deserialize", func(b *testing.B) {
		data, _ := s.Serialize(val)
		var out testhelpers.Person
		b.ResetTimer()
		for b.Loop() {
			_ = s.Deserialize(data, &out)
		}
	})
}
