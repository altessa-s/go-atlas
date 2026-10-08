// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package filter_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/filter"
)

func TestCheckFieldReference(t *testing.T) {
	t.Parallel()

	ident := &filter.IdentNode{Name: "name"}
	tests := []struct {
		name    string
		node    filter.Node
		wantErr error
	}{
		{name: "identifier", node: ident},
		{name: "size call", node: &filter.CallNode{Op: filter.OpSize, Target: ident}},
		{name: "other call is unsupported", node: &filter.CallNode{Op: filter.OpSubstring, Target: ident}, wantErr: filter.ErrUnsupportedOperation},
		{name: "literal is invalid", node: &filter.LiteralNode{Value: "$where"}, wantErr: filter.ErrInvalidExpression},
		{name: "nil is invalid", wantErr: filter.ErrInvalidExpression},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := filter.CheckFieldReference(tc.node)
			if tc.wantErr == nil {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, tc.wantErr)
		})
	}
}
