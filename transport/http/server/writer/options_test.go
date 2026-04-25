// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package writer

import (
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDefaultWriterOptions(t *testing.T) {
	opts := defaultOptions()
	require.Equal(t, DefaultCodecJSON, opts.defaultCodec)
	require.True(t, opts.fallbackOnNegotiationError, "fallbackOnNegotiationError should default to true")
	require.NotNil(t, opts.registry)
	require.NotNil(t, opts.responseBuilder)
}

func TestWithDefaultCodec(t *testing.T) {
	opts := newOptions(WithDefaultCodec("application/xml"))
	require.Equal(t, "application/xml", opts.defaultCodec)
}

func TestWithDefaultCodec_StringPtr(t *testing.T) {
	s := "text/plain"
	opts := newOptions(WithDefaultCodec(&s))
	require.Equal(t, "text/plain", opts.defaultCodec)
}

func TestWithDefaultCodec_NilPtr(t *testing.T) {
	opts := newOptions(WithDefaultCodec[*string](nil))
	require.Equal(t, DefaultCodecJSON, opts.defaultCodec)
}

func TestWithDefaultCodec_Trimmed(t *testing.T) {
	opts := newOptions(WithDefaultCodec("  application/json  "))
	require.Equal(t, "application/json", opts.defaultCodec)
}

func TestWithMaxBodySize(t *testing.T) {
	opts := newOptions(WithMaxBodySize(1024))
	require.Equal(t, int64(1024), opts.maxBodySize)
}

func TestWithWriterLogger(t *testing.T) {
	l := slog.New(slog.DiscardHandler)
	opts := newOptions(WithLogger(l))
	require.Equal(t, l, opts.logger)
}

func TestWithWriterLogger_Nil(t *testing.T) {
	opts := newOptions(WithLogger(nil))
	require.NotNil(t, opts.logger)
}

func TestWithRegistry_Nil(t *testing.T) {
	opts := newOptions(WithRegistry(nil))
	require.NotNil(t, opts.registry)
}

func TestWithErrorConverter(t *testing.T) {
	converter := func(err error) (Error, int) {
		return Error{}, 0
	}
	opts := newOptions(WithErrorConverter(converter))
	require.NotNil(t, opts.errorConverter)
}

func TestWithFallbackOnNegotiationError(t *testing.T) {
	opts := newOptions(WithFallbackOnNegotiationError())
	require.True(t, opts.fallbackOnNegotiationError, "should be true")
}
