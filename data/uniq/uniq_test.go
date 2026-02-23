// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package uniq

import (
	"errors"
	"strings"
	"testing"
)

func TestAdd(t *testing.T) {
	u := NewWithNoop()
	err := u.Add(t.Context(), "key1")
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
}

func TestAddWithValue(t *testing.T) {
	u := NewWithNoop()
	err := u.AddWithValue(t.Context(), "key1", "value1")
	if err != nil {
		t.Fatalf("AddWithValue: %v", err)
	}
}

func TestGetValue(t *testing.T) {
	u := NewWithNoop()
	// Noop provider returns nil for GetValue, which means ErrDoesNotExist.
	var out string
	err := u.GetValue(t.Context(), "key1", &out)
	if !errors.Is(err, ErrDoesNotExist) {
		t.Errorf("expected ErrDoesNotExist, got %v", err)
	}
}

func TestExist(t *testing.T) {
	u := NewWithNoop()
	exists, err := u.Exist(t.Context(), "key1")
	if err != nil {
		t.Fatalf("Exist: %v", err)
	}
	if exists {
		t.Error("expected false for noop")
	}
}

func TestRemove(t *testing.T) {
	u := NewWithNoop()
	err := u.Remove(t.Context(), "key1")
	if err != nil {
		t.Fatalf("Remove: %v", err)
	}
}

func TestClear(t *testing.T) {
	u := NewWithNoop()
	err := u.Clear(t.Context())
	if err != nil {
		t.Fatalf("Clear: %v", err)
	}
}

func TestAdd_EmptyKey(t *testing.T) {
	u := NewWithNoop()
	err := u.Add(t.Context(), "")
	if !errors.Is(err, ErrInvalidKey) {
		t.Errorf("expected ErrInvalidKey, got %v", err)
	}
}

func TestAdd_KeyTooLong(t *testing.T) {
	u := NewWithNoop()
	longKey := strings.Repeat("a", maxKeyLength+1)
	err := u.Add(t.Context(), longKey)
	if !errors.Is(err, ErrInvalidKey) {
		t.Errorf("expected ErrInvalidKey, got %v", err)
	}
}

func TestGetValue_NotFound(t *testing.T) {
	u := NewWithNoop()
	var out string
	err := u.GetValue(t.Context(), "missing", &out)
	if !errors.Is(err, ErrDoesNotExist) {
		t.Errorf("expected ErrDoesNotExist, got %v", err)
	}
}
