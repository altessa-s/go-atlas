// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package driver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNoopDriver_PreRequest(t *testing.T) {
	d := NoopDriver()
	ctx, err := d.PreRequest(t.Context(), httptest.NewRequest("GET", "/", nil))
	require.NoError(t, err)
	require.NotNil(t, ctx)
}

func TestNoopDriver_PostRequest(t *testing.T) {
	d := NoopDriver()
	// Should not panic
	d.PostRequest(t.Context(), ResponseInfo{StatusCode: 200}, httptest.NewRequest("GET", "/", nil), nil)
}

func TestResponseInfo(t *testing.T) {
	ri := ResponseInfo{StatusCode: 200, BytesWritten: 42, HeadersSent: true}
	require.Equal(t, 200, ri.StatusCode)
	require.Equal(t, int64(42), ri.BytesWritten)
	require.True(t, ri.HeadersSent, "HeadersSent should be true")
}

type mockDriver struct {
	preRequestCalled  bool
	postRequestCalled bool
	postResponseInfo  ResponseInfo
}

func (m *mockDriver) PreRequest(ctx context.Context, _ *http.Request) (context.Context, error) {
	m.preRequestCalled = true
	return ctx, nil
}

func (m *mockDriver) PostRequest(_ context.Context, resp ResponseInfo, _ *http.Request, _ error) {
	m.postRequestCalled = true
	m.postResponseInfo = resp
}

type mockDrivenMiddleware struct {
	driver Driver
}

func (m *mockDrivenMiddleware) Name() string { return "mock" }

func (m *mockDrivenMiddleware) DrivenMiddleware(_ context.Context, _ *http.Request) (Driver, context.Context) {
	return m.driver, context.Background()
}

func TestHTTPDrivenMiddleware(t *testing.T) {
	md := &mockDriver{}
	dm := &mockDrivenMiddleware{driver: md}
	mw := HTTPDrivenMiddleware(dm)

	require.Equal(t, "mock", mw.Name())

	handler := mw.Handler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte("hello")) //nolint:errcheck
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/test", nil))

	require.True(t, md.preRequestCalled, "PreRequest should be called")
	require.True(t, md.postRequestCalled, "PostRequest should be called")
	require.Equal(t, http.StatusCreated, md.postResponseInfo.StatusCode)
	require.Equal(t, int64(5), md.postResponseInfo.BytesWritten)
	require.True(t, md.postResponseInfo.HeadersSent, "HeadersSent should be true")
}

func TestHTTPDrivenMiddleware_NilDriver(t *testing.T) {
	dm := &mockDrivenMiddleware{driver: nil}
	mw := HTTPDrivenMiddleware(dm)

	called := false
	handler := mw.Handler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/test", nil))

	require.True(t, called, "handler should be called with nil driver (NoopDriver fallback)")
}

func TestHTTPDrivenMiddleware_PreRequestError(t *testing.T) {
	errDriver := &errorDriver{}
	dm := &mockDrivenMiddleware{driver: errDriver}
	mw := HTTPDrivenMiddleware(dm)

	called := false
	handler := mw.Handler(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		called = true
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/test", nil))

	require.False(t, called, "handler should not be called when PreRequest fails")
	require.True(t, errDriver.postCalled, "PostRequest should still be called after PreRequest error")
}

type errorDriver struct {
	postCalled bool
}

func (e *errorDriver) PreRequest(ctx context.Context, _ *http.Request) (context.Context, error) {
	return ctx, http.ErrAbortHandler
}

func (e *errorDriver) PostRequest(_ context.Context, _ ResponseInfo, _ *http.Request, _ error) {
	e.postCalled = true
}

func TestResponseRecorder_WriteHeader_OnlyOnce(t *testing.T) {
	rec := httptest.NewRecorder()
	rr := &responseRecorder{ResponseWriter: rec, statusCode: http.StatusOK}

	rr.WriteHeader(http.StatusCreated)
	rr.WriteHeader(http.StatusNotFound) // should be ignored

	require.Equal(t, http.StatusCreated, rr.statusCode)
}

func TestResponseRecorder_Write(t *testing.T) {
	rec := httptest.NewRecorder()
	rr := &responseRecorder{ResponseWriter: rec, statusCode: http.StatusOK}

	n, err := rr.Write([]byte("hello"))
	require.NoError(t, err)
	require.Equal(t, 5, n)
	require.Equal(t, int64(5), rr.bytesWritten)
	require.True(t, rr.headersSent, "headersSent should be true after Write")
}

func TestResponseRecorder_Unwrap(t *testing.T) {
	rec := httptest.NewRecorder()
	rr := &responseRecorder{ResponseWriter: rec}
	require.Equal(t, rec, rr.Unwrap())
}
