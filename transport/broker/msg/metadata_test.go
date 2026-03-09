// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package msg

import (
	"testing"
	"time"
)

func TestMeta_Map(t *testing.T) {
	meta := Meta{
		{Key: "k1", Value: "v1"},
		{Key: "k2", Value: "v2"},
	}
	m := meta.Map()
	if m["k1"] != "v1" || m["k2"] != "v2" {
		t.Fatalf("Map() = %v", m)
	}
}

func TestMeta_All(t *testing.T) {
	meta := Meta{
		{Key: "k1", Value: "v1"},
		{Key: "k2", Value: "v2"},
	}
	count := 0
	for k, v := range meta.All() {
		if k == "" || v == "" {
			t.Fatal("empty key or value")
		}
		count++
	}
	if count != 2 {
		t.Fatalf("All() yielded %d items, want 2", count)
	}
}

func TestMeta_Value(t *testing.T) {
	tests := []struct {
		name   string
		meta   Meta
		key    string
		want   string
		wantOk bool
	}{
		{"found", Meta{{Key: "k1", Value: "v1"}}, "k1", "v1", true},
		{"not_found", Meta{{Key: "k1", Value: "v1"}}, "k2", "", false},
		{"empty_meta", nil, "k1", "", false},
		{"first_match", Meta{{Key: "k", Value: "first"}, {Key: "k", Value: "second"}}, "k", "first", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := tt.meta.Value(tt.key)
			if ok != tt.wantOk || got != tt.want {
				t.Fatalf("Value(%q) = (%q, %v), want (%q, %v)", tt.key, got, ok, tt.want, tt.wantOk)
			}
		})
	}
}

func TestMetaFromMap(t *testing.T) {
	m := map[string]string{"k1": "v1", "k2": "v2"}
	meta := MetaFromMap(m)
	if len(meta) != 2 {
		t.Fatalf("MetaFromMap() len = %d, want 2", len(meta))
	}
}

func TestMetaFromMap_Nil(t *testing.T) {
	meta := MetaFromMap(nil)
	if len(meta) != 0 {
		t.Fatalf("MetaFromMap(nil) len = %d, want 0", len(meta))
	}
}

func TestMessageCreatedTimeFromMeta(t *testing.T) {
	now := time.Now().UTC()
	meta := Meta{{Key: MetaKeyMessageCreatedTime, Value: now.Format(MessageCreatedTimeFormat)}}
	got, err := MessageCreatedTimeFromMeta(meta)
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if got.Format(MessageCreatedTimeFormat) != now.Format(MessageCreatedTimeFormat) {
		t.Fatalf("got %v, want %v", got, now)
	}
}

func TestMessageCreatedTimeFromMeta_Nil(t *testing.T) {
	got, err := MessageCreatedTimeFromMeta(nil)
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if !got.IsZero() {
		t.Fatalf("expected zero time, got %v", got)
	}
}

func TestMessageCreatedTimeFromMeta_Missing(t *testing.T) {
	meta := Meta{{Key: "other", Value: "v"}}
	got, err := MessageCreatedTimeFromMeta(meta)
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if !got.IsZero() {
		t.Fatalf("expected zero time, got %v", got)
	}
}

func TestMessageIdFromMeta(t *testing.T) {
	meta := Meta{{Key: MetaKeyMessageId, Value: "msg-123"}}
	if got := MessageIdFromMeta(meta); got != "msg-123" {
		t.Fatalf("MessageIdFromMeta() = %q", got)
	}
}

func TestMessageIdFromMeta_Nil(t *testing.T) {
	if got := MessageIdFromMeta(nil); got != "" {
		t.Fatalf("MessageIdFromMeta(nil) = %q", got)
	}
}

func TestNewMessage_DefaultAckTimeout(t *testing.T) {
	m := NewMessage("t", []byte("x"))
	if m.AckTimeout != NoAckTimeout {
		t.Fatalf("AckTimeout = %v, want NoAckTimeout", m.AckTimeout)
	}
}

func TestNewMessage_WithOptions(t *testing.T) {
	m := NewMessage("t", []byte("x"),
		WithAckTimeout(5*time.Second),
		WithTTL(10*time.Second),
	)
	if m.AckTimeout != 5*time.Second {
		t.Fatalf("AckTimeout = %v", m.AckTimeout)
	}
	if m.TTL != 10*time.Second {
		t.Fatalf("TTL = %v", m.TTL)
	}
}

func TestMessage_Ack_NilAcker(t *testing.T) {
	m := NewMessage("t", []byte("x"))
	if err := m.Ack(); err != nil {
		t.Fatalf("Ack() error = %v", err)
	}
}

func TestMessage_Nak_NilAcker(t *testing.T) {
	m := NewMessage("t", []byte("x"))
	if err := m.Nak(); err != nil {
		t.Fatalf("Nak() error = %v", err)
	}
}

func TestMessage_Term_NilAcker(t *testing.T) {
	m := NewMessage("t", []byte("x"))
	if err := m.Term(); err != nil {
		t.Fatalf("Term() error = %v", err)
	}
}

func TestMessage_InProgress_NilAcker(t *testing.T) {
	m := NewMessage("t", []byte("x"))
	if err := m.InProgress(); err != nil {
		t.Fatalf("InProgress() error = %v", err)
	}
}

func TestMessage_WithAcker(t *testing.T) {
	acked := false
	mock := &mockAcker{ackFn: func() error { acked = true; return nil }}
	m := NewMessage("t", []byte("x"), WithAcker(mock))
	if err := m.Ack(); err != nil {
		t.Fatalf("Ack() error = %v", err)
	}
	if !acked {
		t.Fatal("Ack() not called on acker")
	}
}

type mockAcker struct {
	ackFn func() error
}

func (m *mockAcker) Ack() error                   { return m.ackFn() }
func (m *mockAcker) Nak(_ ...time.Duration) error { return nil }
func (m *mockAcker) Term(_ ...string) error       { return nil }
func (m *mockAcker) InProgress() error            { return nil }
