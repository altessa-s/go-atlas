// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package enumcodec_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/domain/converter"
)

func BenchmarkDomain(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		if _, err := grades.Domain("attrs.grade", protoSenior); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkProto(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		_ = grades.Proto(gradeSenior)
	}
}

func BenchmarkErrorCodec(b *testing.B) {
	type source struct{ Grade protoGrade }
	type destination struct{ Grade grade }
	conv := converter.New[source, *destination](converter.WithErrorCodecs(grades.ErrorCodec()))
	src := source{protoSenior}
	var dst destination
	b.ReportAllocs()
	for b.Loop() {
		conv.Convert(src, &dst)
	}
}
