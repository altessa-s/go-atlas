// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package bufhelpers

import (
	"testing"

	"buf.build/gen/go/bufbuild/protovalidate/protocolbuffers/go/buf/validate"
)

func BenchmarkLeafFieldName(b *testing.B) {
	path := &validate.FieldPath{Elements: []*validate.FieldPathElement{
		{FieldName: new("parent")},
		{FieldName: new("userName")},
	}}
	for b.Loop() {
		_ = leafFieldName(path)
	}
}
