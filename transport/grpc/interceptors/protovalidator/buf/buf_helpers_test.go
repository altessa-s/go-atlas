// Copyright 2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package bufhelpers

import (
	"testing"

	"buf.build/gen/go/bufbuild/protovalidate/protocolbuffers/go/buf/validate"
	"github.com/stretchr/testify/require"

	"google.golang.org/protobuf/proto"
)

func fieldPath(names ...string) *validate.FieldPath {
	elements := make([]*validate.FieldPathElement, 0, len(names))
	for _, name := range names {
		elements = append(elements, &validate.FieldPathElement{FieldName: proto.String(name)})
	}
	return &validate.FieldPath{Elements: elements}
}

func TestLeafFieldName(t *testing.T) {
	tests := []struct {
		name string
		path *validate.FieldPath
		want string
	}{
		{"top_level", fieldPath("userName"), "userName"},
		{"nested_uses_last_segment", fieldPath("parent", "childField"), "childField"},
		{"nil_path", nil, ""},
		{"empty_elements", fieldPath(), ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, leafFieldName(tt.path))
		})
	}
}

func TestWithReasonCode_NilIgnored(t *testing.T) {
	var cfg validatorConfig
	WithReasonCode(nil)(&cfg)
	require.Nil(t, cfg.reasonCode, "nil coder must be ignored")

	WithReasonCode(func(ruleID, fieldName string) string { return "X" })(&cfg)
	require.NotNil(t, cfg.reasonCode)
	require.Equal(t, "X", cfg.reasonCode("any", "any"))
}
