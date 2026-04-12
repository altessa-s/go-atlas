// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package msg

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestMeta_Map(t *testing.T) {
	meta := Meta{
		{Key: "k1", Value: "v1"},
		{Key: "k2", Value: "v2"},
	}
	m := meta.Map()
	require.Equal(t, "v1", m["k1"])
	require.Equal(t, "v2", m["k2"])
}

func TestMeta_All(t *testing.T) {
	meta := Meta{
		{Key: "k1", Value: "v1"},
		{Key: "k2", Value: "v2"},
	}
	count := 0
	for k, v := range meta.All() {
		require.NotEqual(t, "", k)
		require.NotEqual(t, "", v)
		count++
	}
	require.Equal(t, 2, count)
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
			require.Equal(t, tt.wantOk, ok)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestMetaFromMap(t *testing.T) {
	m := map[string]string{"k1": "v1", "k2": "v2"}
	meta := MetaFromMap(m)
	require.Len(t, meta, 2)
}

func TestMetaFromMap_Nil(t *testing.T) {
	meta := MetaFromMap(nil)
	require.Len(t, meta, 0)
}

func TestMessageCreatedTimeFromMeta(t *testing.T) {
	now := time.Now().UTC()
	meta := Meta{{Key: MetaKeyMessageCreatedTime, Value: now.Format(MessageCreatedTimeFormat)}}
	got, err := MessageCreatedTimeFromMeta(meta)
	require.NoError(t, err)
	require.Equal(t, now.Format(MessageCreatedTimeFormat), got.Format(MessageCreatedTimeFormat))
}

func TestMessageCreatedTimeFromMeta_Nil(t *testing.T) {
	got, err := MessageCreatedTimeFromMeta(nil)
	require.NoError(t, err)
	require.True(t, got.IsZero(), "expected zero time, got %v", got)
}

func TestMessageCreatedTimeFromMeta_Missing(t *testing.T) {
	meta := Meta{{Key: "other", Value: "v"}}
	got, err := MessageCreatedTimeFromMeta(meta)
	require.NoError(t, err)
	require.True(t, got.IsZero(), "expected zero time, got %v", got)
}

func TestMessageIdFromMeta(t *testing.T) {
	meta := Meta{{Key: MetaKeyMessageId, Value: "msg-123"}}
	got := MessageIdFromMeta(meta)
	require.Equal(t, "msg-123", got)
}

func TestMessageIdFromMeta_Nil(t *testing.T) {
	got := MessageIdFromMeta(nil)
	require.Equal(t, "", got)
}

func TestNewMessage_DefaultAckTimeout(t *testing.T) {
	m := NewMessage("t", []byte("x"))
	require.Equal(t, NoAckTimeout, m.AckTimeout)
}

func TestNewMessage_WithOptions(t *testing.T) {
	m := NewMessage("t", []byte("x"),
		WithAckTimeout(5*time.Second),
		WithTTL(10*time.Second),
	)
	require.Equal(t, m.AckTimeout, 5*time.Second)
	require.Equal(t, m.TTL, 10*time.Second)
}

func TestMessage_Ack_NilAcker(t *testing.T) {
	m := NewMessage("t", []byte("x"))
	err := m.Ack()
	require.NoError(t, err)
}

func TestMessage_Nak_NilAcker(t *testing.T) {
	m := NewMessage("t", []byte("x"))
	err := m.Nak()
	require.NoError(t, err)
}

func TestMessage_Term_NilAcker(t *testing.T) {
	m := NewMessage("t", []byte("x"))
	err := m.Term()
	require.NoError(t, err)
}

func TestMessage_InProgress_NilAcker(t *testing.T) {
	m := NewMessage("t", []byte("x"))
	err := m.InProgress()
	require.NoError(t, err)
}

func TestMessage_WithAcker(t *testing.T) {
	acked := false
	mock := &mockAcker{ackFn: func() error { acked = true; return nil }}
	m := NewMessage("t", []byte("x"), WithAcker(mock))
	err := m.Ack()
	require.NoError(t, err)
	require.True(t, acked, "Ack() not called on acker")
}

type mockAcker struct {
	ackFn func() error
}

func (m *mockAcker) Ack() error                         { return m.ackFn() }
func (m *mockAcker) Nak(_ ...time.Duration) error       { return nil }
func (m *mockAcker) NakWithBackOff(_ BackOffFunc) error { return nil }
func (m *mockAcker) Term(_ ...string) error             { return nil }
func (m *mockAcker) InProgress() error                  { return nil }
