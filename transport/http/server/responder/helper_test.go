// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package responder

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWriteError_NoWriter(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/test", nil)
	err := WriteError(rec, req, errors.New("fail"), http.StatusBadRequest)
	require.NoError(t, err)
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestWriteError_WithWriter(t *testing.T) {
	w := &mockErrorWriter{}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/test", nil)
	req = req.WithContext(NewContext(req.Context(), w))

	err := WriteError(rec, req, errors.New("fail"), http.StatusBadRequest)
	require.NoError(t, err)
	require.True(t, w.called, "writer should be called")
}
