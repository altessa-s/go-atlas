// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package writer

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	w := New()
	require.NotNil(t, w)
}

func TestWriter_Write_NilResponseWriter(t *testing.T) {
	w := New()
	req := httptest.NewRequest("GET", "/", nil)
	err := w.Write(nil, req, "data")
	require.True(t, errors.Is(err, ErrNilResponseWriter))
}

func TestWriter_Write_NilRequest(t *testing.T) {
	w := New()
	rec := httptest.NewRecorder()
	err := w.Write(rec, nil, "data")
	require.True(t, errors.Is(err, ErrNilRequest))
}

func TestWriter_Write_JSON(t *testing.T) {
	w := New()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Accept", "application/json")

	err := w.Write(rec, req, map[string]string{"key": "value"})
	require.NoError(t, err)

	require.Equal(t, http.StatusOK, rec.Code)

	ct := rec.Header().Get("Content-Type")
	require.True(t, strings.Contains(ct, "application/json"))

	var resp Response
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
}

func TestWriter_Write_DefaultCodec(t *testing.T) {
	w := New()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	// No Accept header - should use default codec

	err := w.Write(rec, req, "hello")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, rec.Code)
}

func TestWriter_Read_NilRequest(t *testing.T) {
	w := New()
	var out struct{}
	err := w.Read(nil, &out)
	require.True(t, errors.Is(err, ErrNilRequest))
}

func TestWriter_Read_NilOutput(t *testing.T) {
	w := New()
	req := httptest.NewRequest("POST", "/", strings.NewReader("{}"))
	err := w.Read(req, nil)
	require.True(t, errors.Is(err, ErrNilOutput))
}

func TestWriter_Read_JSON(t *testing.T) {
	w := New()
	body := `{"name":"test"}`
	req := httptest.NewRequest("POST", "/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	var out map[string]string
	err := w.Read(req, &out)
	require.NoError(t, err)
	require.Equal(t, "test", out["name"])
}

func TestWriter_Read_DefaultContentType(t *testing.T) {
	w := New()
	body := `{"name":"test"}`
	req := httptest.NewRequest("POST", "/", strings.NewReader(body))
	// No Content-Type header - should use default codec

	var out map[string]string
	err := w.Read(req, &out)
	require.NoError(t, err)
}

func TestWriter_Read_UnsupportedContentType(t *testing.T) {
	w := New()
	req := httptest.NewRequest("POST", "/", strings.NewReader("data"))
	req.Header.Set("Content-Type", "application/unsupported-format-xyz")

	var out map[string]string
	err := w.Read(req, &out)
	require.Error(t, err)
}

func TestWriter_WriteError(t *testing.T) {
	w := New()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)

	err := w.WriteError(rec, req, errors.New("something failed"), http.StatusBadRequest)
	require.NoError(t, err)
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestWriter_WriteError_DefaultStatus(t *testing.T) {
	w := New()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)

	err := w.WriteError(rec, req, errors.New("fail"))
	require.NoError(t, err)
	require.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestWriter_Read_MaxBodySize(t *testing.T) {
	w := New(WithMaxBodySize(5))
	req := httptest.NewRequest("POST", "/", strings.NewReader(`{"name":"this is a very long body that exceeds limit"}`))
	req.Header.Set("Content-Type", "application/json")

	var out map[string]string
	err := w.Read(req, &out)
	require.Error(t, err)
}

func TestNewReadWriter(t *testing.T) {
	w := New()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)

	rw := NewReadWriter(rec, req, w)
	require.NotNil(t, rw)
	require.Equal(t, req, rw.Request())
	require.Equal(t, rec, rw.ResponseWriter())
	rw.Release()
}

func TestWriter_NewReadWriter(t *testing.T) {
	w := New()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)

	rw := w.NewReadWriter(rec, req)
	require.NotNil(t, rw)
	rw.Release()
}

func TestReadWriter_Write(t *testing.T) {
	w := New()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)

	rw := NewReadWriter(rec, req, w)
	defer rw.Release()

	err := rw.Write(map[string]string{"key": "value"})
	require.NoError(t, err)
}

func TestReadWriter_Read(t *testing.T) {
	w := New()
	rec := httptest.NewRecorder()
	body := `{"name":"test"}`
	req := httptest.NewRequest("POST", "/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	rw := NewReadWriter(rec, req, w)
	defer rw.Release()

	var out map[string]string
	err := rw.Read(&out)
	require.NoError(t, err)
}

func TestReadWriter_WriteError(t *testing.T) {
	w := New()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)

	rw := NewReadWriter(rec, req, w)
	defer rw.Release()

	err := rw.WriteError(errors.New("fail"), http.StatusBadRequest)
	require.NoError(t, err)
}

func TestReadWriter_Release_ClearsFields(t *testing.T) {
	w := New()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)

	rw := NewReadWriter(rec, req, w)
	rw.Release()
	// After release, the underlying struct fields should be nil.
	// We can't check directly, but at least verify no panic.
}

func TestWriter_Write_WithAcceptWildcard(t *testing.T) {
	w := New()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Accept", "*/*")

	err := w.Write(rec, req, "data")
	require.NoError(t, err)
}

func TestWriter_Read_EmptyBody(t *testing.T) {
	w := New()
	req := httptest.NewRequest("POST", "/", strings.NewReader(""))
	req.Header.Set("Content-Type", "application/json")
	req.Body = io.NopCloser(strings.NewReader(""))

	var out map[string]string
	err := w.Read(req, &out)
	// Empty body should produce an unmarshal error
	require.Error(t, err)
}
