// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package codec

import (
	"errors"
	"testing"
)

type mockCodec struct {
	mime string
}

func (m *mockCodec) Encode(data any) ([]byte, error)   { return []byte("encoded"), nil }
func (m *mockCodec) Decode(data []byte, out any) error { return nil }
func (m *mockCodec) ContentType() string               { return m.mime }

func TestNewRegistry(t *testing.T) {
	r := NewRegistry()
	if r == nil {
		t.Fatal("NewRegistry() returned nil")
	}
}

func TestRegistry_RegisterEncoder(t *testing.T) {
	r := NewRegistry()
	enc := &mockCodec{mime: "test/plain"}
	if err := r.RegisterEncoder("test/plain", enc); err != nil {
		t.Fatalf("RegisterEncoder() error = %v", err)
	}
}

func TestRegistry_RegisterEncoder_Nil(t *testing.T) {
	r := NewRegistry()
	if err := r.RegisterEncoder("test/plain", nil); err == nil {
		t.Fatal("expected error for nil encoder")
	}
}

func TestRegistry_RegisterEncoder_Duplicate(t *testing.T) {
	r := NewRegistry()
	enc := &mockCodec{mime: "test/dup"}
	r.RegisterEncoder("test/dup", enc)
	err := r.RegisterEncoder("test/dup", enc)
	if !errors.Is(err, ErrCodecAlreadyRegistered) {
		t.Fatalf("expected ErrCodecAlreadyRegistered, got %v", err)
	}
}

func TestRegistry_RegisterDecoder(t *testing.T) {
	r := NewRegistry()
	dec := &mockCodec{mime: "test/plain"}
	if err := r.RegisterDecoder("test/plain", dec); err != nil {
		t.Fatalf("RegisterDecoder() error = %v", err)
	}
}

func TestRegistry_RegisterDecoder_Nil(t *testing.T) {
	r := NewRegistry()
	if err := r.RegisterDecoder("test/plain", nil); err == nil {
		t.Fatal("expected error for nil decoder")
	}
}

func TestRegistry_RegisterDecoder_Duplicate(t *testing.T) {
	r := NewRegistry()
	dec := &mockCodec{mime: "test/dup"}
	r.RegisterDecoder("test/dup", dec)
	err := r.RegisterDecoder("test/dup", dec)
	if !errors.Is(err, ErrCodecAlreadyRegistered) {
		t.Fatalf("expected ErrCodecAlreadyRegistered, got %v", err)
	}
}

func TestRegistry_RegisterCodec(t *testing.T) {
	r := NewRegistry()
	c := &mockCodec{mime: "test/codec"}
	if err := r.RegisterCodec(c); err != nil {
		t.Fatalf("RegisterCodec() error = %v", err)
	}

	enc, ok := r.GetEncoder("test/codec")
	if !ok || enc == nil {
		t.Fatal("encoder not found")
	}
	dec, ok := r.GetDecoder("test/codec")
	if !ok || dec == nil {
		t.Fatal("decoder not found")
	}
}

func TestRegistry_RegisterCodec_Nil(t *testing.T) {
	r := NewRegistry()
	if err := r.RegisterCodec(nil); err == nil {
		t.Fatal("expected error for nil codec")
	}
}

func TestRegistry_RegisterCodec_DuplicateDecoderRollback(t *testing.T) {
	r := NewRegistry()
	c := &mockCodec{mime: "test/rollback"}

	// Pre-register decoder to force rollback
	r.RegisterDecoder("test/rollback", c)

	err := r.RegisterCodec(c)
	if err == nil {
		t.Fatal("expected error for duplicate decoder")
	}

	// Encoder registration should have been rolled back
	_, ok := r.GetEncoder("test/rollback")
	if ok {
		t.Fatal("encoder should have been rolled back")
	}
}

func TestRegistry_GetEncoder_NotFound(t *testing.T) {
	r := NewRegistry()
	_, ok := r.GetEncoder("nonexistent/type")
	if ok {
		t.Fatal("should not find nonexistent encoder")
	}
}

func TestRegistry_GetDecoder_NotFound(t *testing.T) {
	r := NewRegistry()
	_, ok := r.GetDecoder("nonexistent/type")
	if ok {
		t.Fatal("should not find nonexistent decoder")
	}
}

func TestRegistry_Negotiate(t *testing.T) {
	r := NewRegistry()
	r.RegisterEncoder("application/json", &mockCodec{mime: "application/json"})

	enc, mime, err := r.Negotiate("application/json")
	if err != nil {
		t.Fatalf("Negotiate() error = %v", err)
	}
	if enc == nil {
		t.Fatal("encoder is nil")
	}
	if mime != "application/json" {
		t.Fatalf("mime = %q", mime)
	}
}

func TestRegistry_Negotiate_EmptyHeader(t *testing.T) {
	r := NewRegistry()
	_, _, err := r.Negotiate("")
	if !errors.Is(err, ErrNoCodecFound) {
		t.Fatalf("expected ErrNoCodecFound, got %v", err)
	}
}

func TestRegistry_Negotiate_NoMatch(t *testing.T) {
	r := NewRegistry()
	r.RegisterEncoder("application/json", &mockCodec{mime: "application/json"})

	_, _, err := r.Negotiate("application/xml")
	if err == nil {
		t.Fatal("expected error for no matching codec")
	}
}

func TestRegistry_ListEncoders(t *testing.T) {
	r := NewRegistry()
	r.RegisterEncoder("test/a", &mockCodec{mime: "test/a"})
	r.RegisterEncoder("test/b", &mockCodec{mime: "test/b"})

	count := 0
	for range r.ListEncoders() {
		count++
	}
	if count != 2 {
		t.Fatalf("ListEncoders() yielded %d, want 2", count)
	}
}

func TestRegistry_ListDecoders(t *testing.T) {
	r := NewRegistry()
	r.RegisterDecoder("test/a", &mockCodec{mime: "test/a"})
	r.RegisterDecoder("test/b", &mockCodec{mime: "test/b"})

	count := 0
	for range r.ListDecoders() {
		count++
	}
	if count != 2 {
		t.Fatalf("ListDecoders() yielded %d, want 2", count)
	}
}

func TestRegistry_Encoders(t *testing.T) {
	r := NewRegistry()
	r.RegisterEncoder("test/a", &mockCodec{mime: "test/a"})

	count := 0
	for mime, enc := range r.Encoders() {
		if mime == "" || enc == nil {
			t.Fatal("empty mime or nil encoder")
		}
		count++
	}
	if count != 1 {
		t.Fatalf("Encoders() yielded %d", count)
	}
}

func TestRegistry_Decoders(t *testing.T) {
	r := NewRegistry()
	r.RegisterDecoder("test/a", &mockCodec{mime: "test/a"})

	count := 0
	for mime, dec := range r.Decoders() {
		if mime == "" || dec == nil {
			t.Fatal("empty mime or nil decoder")
		}
		count++
	}
	if count != 1 {
		t.Fatalf("Decoders() yielded %d", count)
	}
}

func TestRegistry_MimeTypeNormalization(t *testing.T) {
	r := NewRegistry()
	enc := &mockCodec{mime: "test/norm"}
	r.RegisterEncoder("  Test/Norm ; charset=utf-8  ", enc)

	got, ok := r.GetEncoder("test/norm")
	if !ok || got == nil {
		t.Fatal("should find encoder with normalized mime type")
	}
}

func TestDefaultRegistry(t *testing.T) {
	r := DefaultRegistry()
	if r == nil {
		t.Fatal("DefaultRegistry() returned nil")
	}
	// Should have JSON and XML codecs from init()
	if _, ok := r.GetEncoder("application/json"); !ok {
		t.Fatal("default registry should have JSON encoder")
	}
	if _, ok := r.GetEncoder("application/xml"); !ok {
		t.Fatal("default registry should have XML encoder")
	}
}
