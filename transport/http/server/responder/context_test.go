// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package responder

import (
	"net/http"
	"testing"
)

type mockErrorWriter struct {
	called bool
}

func (m *mockErrorWriter) WriteError(w http.ResponseWriter, r *http.Request, err error, statusCode ...int) error {
	m.called = true
	return nil
}

func TestNewContext_FromContext(t *testing.T) {
	w := &mockErrorWriter{}
	ctx := NewContext(t.Context(), w)
	got := FromContext(ctx)
	if got != w {
		t.Fatal("expected same writer")
	}
}

func TestFromContext_Nil(t *testing.T) {
	got := FromContext(t.Context())
	if got != nil {
		t.Fatal("expected nil")
	}
}
