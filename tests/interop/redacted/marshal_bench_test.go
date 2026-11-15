// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redacted_test

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
	"gopkg.in/yaml.v3"

	"github.com/altessa-s/go-atlas-core/types/redacted"
)

func BenchmarkUnmarshalYAML(b *testing.B) {
	data := []byte("super-secret\n")
	var s redacted.RedactedString
	for b.Loop() {
		_ = yaml.Unmarshal(data, &s)
	}
}

func BenchmarkUnmarshalBSONValue(b *testing.B) {
	typ, data, err := bson.MarshalValue("super-secret")
	if err != nil {
		b.Fatal(err)
	}
	var s redacted.RedactedString
	for b.Loop() {
		_ = s.UnmarshalBSONValue(byte(typ), data)
	}
}
