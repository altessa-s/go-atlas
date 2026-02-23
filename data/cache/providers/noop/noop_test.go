// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package noop

import (
	"errors"
	"testing"
	"time"

	"github.com/altessa-s/go-atlas/data/cache/providers"
)

func TestNoop_Save(t *testing.T) {
	p := New()
	err := p.Save(t.Context(), "key", []byte("value"), time.Minute)
	if err != nil {
		t.Errorf("Save: %v", err)
	}
}

func TestNoop_Get_ReturnsErrMissing(t *testing.T) {
	p := New()
	_, err := p.Get(t.Context(), "key")
	if !errors.Is(err, providers.ErrMissing) {
		t.Errorf("Get: got %v, want ErrMissing", err)
	}
}

func TestNoop_Exists_ReturnsFalse(t *testing.T) {
	p := New()
	exists, err := p.Exists(t.Context(), "key")
	if err != nil {
		t.Errorf("Exists: %v", err)
	}
	if exists {
		t.Error("expected false")
	}
}

func TestNoop_Delete(t *testing.T) {
	p := New()
	err := p.Delete(t.Context(), "key")
	if err != nil {
		t.Errorf("Delete: %v", err)
	}
}

func TestNoop_DeleteMany(t *testing.T) {
	p := New()
	err := p.DeleteMany(t.Context(), "k1", "k2")
	if err != nil {
		t.Errorf("DeleteMany: %v", err)
	}
}
