// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package optional_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/core/types/optional"
)

func BenchmarkMarshalJSON_Some(b *testing.B) {
	opt := optional.Some("hello")
	var (
		raw []byte
		err error
	)
	for b.Loop() {
		raw, err = opt.MarshalJSON()
	}
	_, _ = raw, err
}

func BenchmarkMarshalJSON_None(b *testing.B) {
	opt := optional.None[string]()
	var (
		raw []byte
		err error
	)
	for b.Loop() {
		raw, err = opt.MarshalJSON()
	}
	_, _ = raw, err
}

func BenchmarkUnmarshalJSON_Some(b *testing.B) {
	in := []byte(`"hello"`)
	var opt optional.Optional[string]
	for b.Loop() {
		if err := opt.UnmarshalJSON(in); err != nil {
			require.NoError(b, err)
		}
	}
}

func BenchmarkUnmarshalJSON_Null(b *testing.B) {
	in := []byte("null")
	var opt optional.Optional[string]
	for b.Loop() {
		if err := opt.UnmarshalJSON(in); err != nil {
			require.NoError(b, err)
		}
	}
}
