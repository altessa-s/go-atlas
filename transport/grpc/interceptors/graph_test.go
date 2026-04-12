// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package interceptors

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetInterceptorName(t *testing.T) {
	tests := []struct {
		name string
		item any
		want string
	}{
		{"named", &NoOpInterceptor{}, "noop"},
		{"unnamed", "string", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getInterceptorName(tt.item)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestOrderByDependencies_Empty(t *testing.T) {
	result, err := orderByDependencies(nil, nil)
	require.NoError(t, err)
	require.Nil(t, result)
}

func TestWrapUnwrapItems(t *testing.T) {
	items := []any{&NoOpInterceptor{}, &NoOpClientInterceptor{}}
	wrapped := wrapItems(items)
	require.Len(t, wrapped, 2)
	require.Equal(t, "noop", wrapped[0].Name())
	unwrapped := unwrapItems(wrapped)
	require.Len(t, unwrapped, 2)
}
