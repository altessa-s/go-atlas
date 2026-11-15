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
	t.Parallel()
	d := NoopDriver()
	ctx, err := d.PreRequest(t.Context(), httptest.NewRequest("GET", "/", nil))
	require.NoError(t, err)
	require.NotNil(t, ctx)
}

func TestNoopDriver_PostRequest(t *testing.T) {
	t.Parallel()
	d := NoopDriver()
	// Should not panic
	d.PostRequest(t.Context(), ResponseInfo{StatusCode: 200}, httptest.NewRequest("GET", "/", nil), nil)
}

func TestResponseInfo(t *testing.T) {
	t.Parallel()
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

func (m *mockDrivenMiddleware) DrivenMiddleware(ctx context.Context, _ *http.Request) (Driver, context.Context) {
	return m.driver, ctx
}

func TestHTTPDrivenMiddleware(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	require.Equal(t, http.StatusInternalServerError, rec.Code, "client must see the failure status")
	require.Equal(t, http.StatusInternalServerError, errDriver.info.StatusCode)
	require.True(t, errDriver.info.HeadersSent)
	require.Positive(t, errDriver.info.BytesWritten)
	require.ErrorIs(t, errDriver.preErr, http.ErrAbortHandler)
}

type errorDriver struct {
	postCalled bool
	info       ResponseInfo
	preErr     error
}

func (e *errorDriver) PreRequest(ctx context.Context, _ *http.Request) (context.Context, error) {
	return ctx, http.ErrAbortHandler
}

func (e *errorDriver) PostRequest(_ context.Context, info ResponseInfo, _ *http.Request, err error) {
	e.postCalled = true
	e.info = info
	e.preErr = err
}

func TestHTTPDrivenMiddlewareInformationalResponse(t *testing.T) {
	t.Parallel()
	md := &mockDriver{}
	handler := HTTPDrivenMiddleware(&mockDrivenMiddleware{driver: md}).Handler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusEarlyHints)
		w.WriteHeader(http.StatusCreated)
	}))
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	require.Equal(t, http.StatusCreated, md.postResponseInfo.StatusCode)
	require.True(t, md.postResponseInfo.HeadersSent)
}
