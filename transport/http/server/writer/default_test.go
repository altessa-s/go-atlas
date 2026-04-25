// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package writer

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewDefault(t *testing.T) {
	d := NewDefault()
	require.NotNil(t, d)
}

func TestDefault_Build_Data(t *testing.T) {
	d := NewDefault()
	req := httptest.NewRequest("GET", "/", nil)

	resp, status := d.Build(req, map[string]string{"key": "value"})
	require.Equal(t, http.StatusOK, status)
	r, ok := resp.(*Response)
	require.True(t, ok)
	require.NotNil(t, r.Data)
	require.Nil(t, r.Error)
}

func TestDefault_Build_Error(t *testing.T) {
	d := NewDefault()
	req := httptest.NewRequest("GET", "/", nil)

	resp, status := d.Build(req, errors.New("something failed"))
	require.Equal(t, http.StatusInternalServerError, status)
	r, ok := resp.(*Response)
	require.True(t, ok)
	require.NotNil(t, r.Error)
	require.Equal(t, "", r.Error.Code)
	// Plain errors never expose err.Error() — the response must carry the
	// generic HTTP status text instead. err.Error() may contain DB driver
	// output, file paths or other internal details.
	require.Equal(t, http.StatusText(http.StatusInternalServerError), r.Error.Message)
}

// TestDefault_Build_Error_DoesNotLeakRawError is a regression test: an
// error whose Error() looks like it came from a database driver must NOT
// surface in the response body. Only callers that explicitly implement
// [Messager] opt in to having their message exposed.
func TestDefault_Build_Error_DoesNotLeakRawError(t *testing.T) {
	d := NewDefault()
	req := httptest.NewRequest("GET", "/", nil)

	const sensitive = `pq: relation "users_secret_keys" does not exist; conn=postgres://app:p4ssw0rd@db.internal:5432/app?sslmode=require`

	resp, status := d.Build(req, errors.New(sensitive))
	require.Equal(t, http.StatusInternalServerError, status)
	r, ok := resp.(*Response)
	require.True(t, ok)
	require.NotContains(t, r.Error.Message, "pq:")
	require.NotContains(t, r.Error.Message, "p4ssw0rd")
	require.NotContains(t, r.Error.Message, "db.internal")
	require.Equal(t, http.StatusText(http.StatusInternalServerError), r.Error.Message)
}

type customError struct {
	code    string
	message string
	status  int
}

func (e *customError) Error() string   { return "internal: " + e.message }
func (e *customError) Code() string    { return e.code }
func (e *customError) Message() string { return e.message }
func (e *customError) HTTPStatus() int { return e.status }

func TestDefault_Build_ErrorWithInterfaces(t *testing.T) {
	d := NewDefault()
	req := httptest.NewRequest("GET", "/", nil)

	err := &customError{code: "VALIDATION", message: "invalid input", status: http.StatusBadRequest}
	resp, status := d.Build(req, err)
	require.Equal(t, http.StatusBadRequest, status)
	r := resp.(*Response)
	require.Equal(t, "VALIDATION", r.Error.Code)
	require.Equal(t, "invalid input", r.Error.Message)
}

func TestDefault_Build_ErrorWithConverter(t *testing.T) {
	converter := func(err error) (Error, int) {
		return Error{Code: "CUSTOM", Message: err.Error()}, http.StatusUnprocessableEntity
	}
	d := NewDefault(WithDefaultBuilderErrorConverter(converter))
	req := httptest.NewRequest("GET", "/", nil)

	resp, status := d.Build(req, errors.New("converted"))
	require.Equal(t, http.StatusUnprocessableEntity, status)
	r := resp.(*Response)
	require.Equal(t, "CUSTOM", r.Error.Code)
}

func TestDefault_Build_ErrorConverterZeroStatus(t *testing.T) {
	converter := func(err error) (Error, int) {
		return Error{Code: "TEST", Message: err.Error()}, 0
	}
	d := NewDefault(WithDefaultBuilderErrorConverter(converter))
	req := httptest.NewRequest("GET", "/", nil)

	_, status := d.Build(req, errors.New("test"))
	require.Equal(t, http.StatusInternalServerError, status)
}

func TestDefault_Build_ErrorWithEmptyCode(t *testing.T) {
	d := NewDefault()
	req := httptest.NewRequest("GET", "/", nil)

	err := &emptyCodeError{}
	resp, _ := d.Build(req, err)
	r := resp.(*Response)
	require.Equal(t, "", r.Error.Code)
}

type emptyCodeError struct{}

func (e *emptyCodeError) Error() string { return "error" }
func (e *emptyCodeError) Code() string  { return "" }

func TestDefault_Build_ErrorWithEmptyMessage(t *testing.T) {
	d := NewDefault()
	req := httptest.NewRequest("GET", "/", nil)

	err := &emptyMessageError{}
	resp, _ := d.Build(req, err)
	r := resp.(*Response)
	// Empty Messager() output falls back to the generic HTTP status text,
	// not err.Error() (which could carry sensitive details).
	require.Equal(t, http.StatusText(http.StatusInternalServerError), r.Error.Message)
}

type emptyMessageError struct{}

func (e *emptyMessageError) Error() string   { return "fallback error" }
func (e *emptyMessageError) Message() string { return "" }

func TestDefault_Build_ErrorWithZeroStatus(t *testing.T) {
	d := NewDefault()
	req := httptest.NewRequest("GET", "/", nil)

	err := &zeroStatusError{}
	_, status := d.Build(req, err)
	require.Equal(t, http.StatusInternalServerError, status)
}

type zeroStatusError struct{}

func (e *zeroStatusError) Error() string   { return "error" }
func (e *zeroStatusError) HTTPStatus() int { return 0 }
