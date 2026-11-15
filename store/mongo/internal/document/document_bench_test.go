// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package document_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/store/mongo/internal/document"
)

func BenchmarkConvert(b *testing.B) {
	engine := document.New(document.NewParser(), nil)
	entity := struct {
		Name   string `bson:"name"`
		Active bool   `bson:"active"`
	}{"example", true}
	for b.Loop() {
		if _, _, err := engine.Convert(b.Context(), entity, false); err != nil {
			b.Fatal(err)
		}
	}
}
