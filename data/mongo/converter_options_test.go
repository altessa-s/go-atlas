// Copyright 2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"reflect"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/domain/converter"

	convcodec "github.com/altessa-s/go-atlas/domain/converter/codec"
)

func TestWithConverterOptions_StoresOptions(t *testing.T) {
	t.Parallel()

	m, err := New("testdb",
		WithConverterOptions(converter.WithIgnoreZeroValues()),
	)
	require.NoError(t, err)
	require.Len(t, m.config.ConverterOptions, 1)
}

func TestWithConverterOptions_AccumulatesAcrossCalls(t *testing.T) {
	t.Parallel()

	m, err := New("testdb",
		WithConverterOptions(converter.WithIgnoreZeroValues()),
		WithConverterOptions(converter.WithIgnoreNilValues()),
	)
	require.NoError(t, err)
	require.Len(t, m.config.ConverterOptions, 2)
}

func TestWithConverterOptions_DefaultEmpty(t *testing.T) {
	t.Parallel()

	m, err := New("testdb")
	require.NoError(t, err)
	require.Empty(t, m.config.ConverterOptions)
}

// TestWithConverterOptions_PassedToConverter exercises the same call shape used
// inside GetEntity / GetEntities: prepend WithHandleEmbeddedStructs(true) to
// the stored options and pass to NewShared. A fake codec records invocations
// so we can confirm the user-provided option reaches the converter. The codec
// chain is only consulted when source and destination field types differ, so
// the test uses int → string to force routing through the chain.
func TestWithConverterOptions_PassedToConverter(t *testing.T) {
	t.Parallel()

	var calls atomic.Int64
	spy := func(fieldName string, src, dst reflect.Value, next convcodec.CodecHandler) {
		calls.Add(1)
		next(fieldName, src, dst)
	}

	m, err := New("testdb",
		WithConverterOptions(converter.WithCodecs(spy)),
	)
	require.NoError(t, err)

	type src struct{ Value int }
	type dst struct{ Value string }

	convOpts := append(
		[]converter.Option{converter.WithHandleEmbeddedStructs(true)},
		m.config.ConverterOptions...,
	)
	conv := converter.NewShared[src, *dst](convOpts...)

	var d dst
	conv.Convert(src{Value: 42}, &d)
	require.Positive(t, calls.Load(), "spy codec must be invoked by the converter")
}
