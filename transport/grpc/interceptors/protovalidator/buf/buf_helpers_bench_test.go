// Copyright 2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package bufhelpers

import (
	"testing"

	"buf.build/gen/go/bufbuild/protovalidate/protocolbuffers/go/buf/validate"

	"google.golang.org/protobuf/proto"
)

func BenchmarkBuildErrorCode_Required(b *testing.B) {
	path := &validate.FieldPath{Elements: []*validate.FieldPathElement{
		{FieldName: proto.String("userName")},
	}}
	for b.Loop() {
		_ = BuildErrorCode("required", path)
	}
}

func BenchmarkBuildErrorCode_Standard(b *testing.B) {
	for b.Loop() {
		_ = BuildErrorCode("int64.gte", nil)
	}
}
