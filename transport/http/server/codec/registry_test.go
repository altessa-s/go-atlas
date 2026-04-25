// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package codec

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

type mockCodec struct {
	mime string
}

func (m *mockCodec) Encode(data any) ([]byte, error)   { return []byte("encoded"), nil }
func (m *mockCodec) Decode(data []byte, out any) error { return nil }
func (m *mockCodec) ContentType() string               { return m.mime }

func TestNewRegistry(t *testing.T) {
	r := NewRegistry()
	require.NotNil(t, r)
}

func TestRegistry_RegisterEncoder(t *testing.T) {
	r := NewRegistry()
	enc := &mockCodec{mime: "test/plain"}
	err := r.RegisterEncoder("test/plain", enc)
	require.NoError(t, err)
}

func TestRegistry_RegisterEncoder_Nil(t *testing.T) {
	r := NewRegistry()
	err := r.RegisterEncoder("test/plain", nil)
	require.Error(t, err)
}

func TestRegistry_RegisterEncoder_Duplicate(t *testing.T) {
	r := NewRegistry()
	enc := &mockCodec{mime: "test/dup"}
	r.RegisterEncoder("test/dup", enc)
	err := r.RegisterEncoder("test/dup", enc)
	require.True(t, errors.Is(err, ErrCodecAlreadyRegistered))
}

func TestRegistry_RegisterDecoder(t *testing.T) {
	r := NewRegistry()
	dec := &mockCodec{mime: "test/plain"}
	err := r.RegisterDecoder("test/plain", dec)
	require.NoError(t, err)
}

func TestRegistry_RegisterDecoder_Nil(t *testing.T) {
	r := NewRegistry()
	err := r.RegisterDecoder("test/plain", nil)
	require.Error(t, err)
}

func TestRegistry_RegisterDecoder_Duplicate(t *testing.T) {
	r := NewRegistry()
	dec := &mockCodec{mime: "test/dup"}
	r.RegisterDecoder("test/dup", dec)
	err := r.RegisterDecoder("test/dup", dec)
	require.True(t, errors.Is(err, ErrCodecAlreadyRegistered))
}

func TestRegistry_RegisterCodec(t *testing.T) {
	r := NewRegistry()
	c := &mockCodec{mime: "test/codec"}
	err := r.RegisterCodec(c)
	require.NoError(t, err)

	enc, ok := r.GetEncoder("test/codec")
	require.True(t, ok)
	require.NotNil(t, enc)
	dec, ok := r.GetDecoder("test/codec")
	require.True(t, ok)
	require.NotNil(t, dec)
}

func TestRegistry_RegisterCodec_Nil(t *testing.T) {
	r := NewRegistry()
	err := r.RegisterCodec(nil)
	require.Error(t, err)
}

func TestRegistry_RegisterCodec_DuplicateDecoderRollback(t *testing.T) {
	r := NewRegistry()
	c := &mockCodec{mime: "test/rollback"}

	// Pre-register decoder to force rollback
	r.RegisterDecoder("test/rollback", c)

	err := r.RegisterCodec(c)
	require.Error(t, err)

	// Encoder registration should have been rolled back
	_, ok := r.GetEncoder("test/rollback")
	require.False(t, ok, "encoder should have been rolled back")
}

func TestRegistry_GetEncoder_NotFound(t *testing.T) {
	r := NewRegistry()
	_, ok := r.GetEncoder("nonexistent/type")
	require.False(t, ok, "should not find nonexistent encoder")
}

func TestRegistry_GetDecoder_NotFound(t *testing.T) {
	r := NewRegistry()
	_, ok := r.GetDecoder("nonexistent/type")
	require.False(t, ok, "should not find nonexistent decoder")
}

func TestRegistry_Negotiate(t *testing.T) {
	r := NewRegistry()
	r.RegisterEncoder("application/json", &mockCodec{mime: "application/json"})

	enc, mime, err := r.Negotiate("application/json")
	require.NoError(t, err)
	require.NotNil(t, enc)
	require.Equal(t, "application/json", mime)
}

func TestRegistry_Negotiate_EmptyHeader(t *testing.T) {
	r := NewRegistry()
	_, _, err := r.Negotiate("")
	require.True(t, errors.Is(err, ErrNoCodecFound))
}

func TestRegistry_Negotiate_NoMatch(t *testing.T) {
	r := NewRegistry()
	r.RegisterEncoder("application/json", &mockCodec{mime: "application/json"})

	_, _, err := r.Negotiate("application/xml")
	require.Error(t, err)
}

func TestRegistry_ListEncoders(t *testing.T) {
	r := NewRegistry()
	r.RegisterEncoder("test/a", &mockCodec{mime: "test/a"})
	r.RegisterEncoder("test/b", &mockCodec{mime: "test/b"})

	count := 0
	for range r.ListEncoders() {
		count++
	}
	require.Equal(t, 2, count)
}

func TestRegistry_ListDecoders(t *testing.T) {
	r := NewRegistry()
	r.RegisterDecoder("test/a", &mockCodec{mime: "test/a"})
	r.RegisterDecoder("test/b", &mockCodec{mime: "test/b"})

	count := 0
	for range r.ListDecoders() {
		count++
	}
	require.Equal(t, 2, count)
}

func TestRegistry_Encoders(t *testing.T) {
	r := NewRegistry()
	r.RegisterEncoder("test/a", &mockCodec{mime: "test/a"})

	count := 0
	for mime, enc := range r.Encoders() {
		require.NotEqual(t, "", mime, "empty mime")
		require.NotNil(t, enc, "nil encoder")
		count++
	}
	require.Equal(t, 1, count)
}

func TestRegistry_Decoders(t *testing.T) {
	r := NewRegistry()
	r.RegisterDecoder("test/a", &mockCodec{mime: "test/a"})

	count := 0
	for mime, dec := range r.Decoders() {
		require.NotEqual(t, "", mime, "empty mime")
		require.NotNil(t, dec, "nil decoder")
		count++
	}
	require.Equal(t, 1, count)
}

func TestRegistry_MimeTypeNormalization(t *testing.T) {
	r := NewRegistry()
	enc := &mockCodec{mime: "test/norm"}
	r.RegisterEncoder("  Test/Norm ; charset=utf-8  ", enc)

	got, ok := r.GetEncoder("test/norm")
	require.True(t, ok, "should find encoder with normalized mime type")
	require.NotNil(t, got)
}

func TestDefaultRegistry(t *testing.T) {
	r := DefaultRegistry()
	require.NotNil(t, r)
	// Should have JSON and XML codecs from init()
	_, ok := r.GetEncoder("application/json")
	require.True(t, ok, "default registry should have JSON encoder")
	_, ok = r.GetEncoder("application/xml")
	require.True(t, ok, "default registry should have XML encoder")
}
