// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package bufhelpers

import (
	"testing"

	"buf.build/gen/go/bufbuild/protovalidate/protocolbuffers/go/buf/validate"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/protovalidator/reasoncode"

	"google.golang.org/protobuf/proto"
)

func fieldPath(names ...string) *validate.FieldPath {
	elements := make([]*validate.FieldPathElement, 0, len(names))
	for _, name := range names {
		elements = append(elements, &validate.FieldPathElement{FieldName: proto.String(name)})
	}
	return &validate.FieldPath{Elements: elements}
}

func TestBuildErrorCode_Required(t *testing.T) {
	tests := []struct {
		name string
		path *validate.FieldPath
		want string
	}{
		{"top_level", fieldPath("userName"), "USER_NAME_REQUIRED"},
		{"nested_uses_last_segment", fieldPath("parent", "childField"), "CHILD_FIELD_REQUIRED"},
		{"nil_path_falls_back", nil, "REQUIRED"},
		{"empty_elements_fall_back", fieldPath(), "REQUIRED"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, BuildErrorCode("required", tt.path))
		})
	}
}

func TestBuildErrorCode_DefaultResolver(t *testing.T) {
	tests := []struct {
		ruleID string
		want   string
	}{
		{"", ""},
		{"int64.gte", reasoncode.InvalidMinLengthOrValue},
		{"int64.lte", reasoncode.InvalidMaxLengthOrValue},
		{"string.email", reasoncode.InvalidFormatEmail},
		// The default resolver has no catalog: an unmapped rule resolves to
		// Unknown rather than leaking the raw rule ID.
		{"acme.string.thing", reasoncode.Unknown},
		{"no.such.rule", reasoncode.Unknown},
	}
	for _, tt := range tests {
		t.Run(tt.ruleID, func(t *testing.T) {
			require.Equal(t, tt.want, BuildErrorCode(tt.ruleID, nil))
		})
	}
}
