// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package msg

import (
	"testing"
	"time"
)

func TestNewMessageWithMeta_CreatedTimeCanBeOverridden(t *testing.T) {
	override := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC).Format(MessageCreatedTimeFormat)

	m := NewMessageWithMeta("t", []byte("x"), Meta{
		{Key: MetaKeyMessageCreatedTime, Value: override},
	})

	got, ok := m.Metadata.Value(MetaKeyMessageCreatedTime)
	if !ok {
		t.Fatalf("missing %s", MetaKeyMessageCreatedTime)
	}
	if got != override {
		t.Fatalf("created_time=%q, want %q", got, override)
	}
}

func TestNewMessageWithMeta_DeduplicatesByKeyPreservingFirst(t *testing.T) {
	m := NewMessageWithMeta("t", []byte("x"), Meta{
		{Key: "k", Value: "1"},
		{Key: "k", Value: "2"},
	})

	got, ok := m.Metadata.Value("k")
	if !ok {
		t.Fatalf("missing key k")
	}
	if got != "1" {
		t.Fatalf("k=%q, want %q", got, "1")
	}
}

func TestNewMessageWithMeta_DataIsCopied(t *testing.T) {
	in := []byte("hello")
	m := NewMessageWithMeta("t", in, nil)
	in[0] = 'H'
	if string(m.Data) != "hello" {
		t.Fatalf("data=%q, want %q", string(m.Data), "hello")
	}
}
