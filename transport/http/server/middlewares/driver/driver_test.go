// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package driver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNoopDriver_PreRequest(t *testing.T) {
	d := NoopDriver()
	ctx, err := d.PreRequest(t.Context(), httptest.NewRequest("GET", "/", nil))
	if err != nil {
		t.Fatal(err)
	}
	if ctx == nil {
		t.Fatal("context should not be nil")
	}
}

func TestNoopDriver_PostRequest(t *testing.T) {
	d := NoopDriver()
	// Should not panic
	d.PostRequest(t.Context(), ResponseInfo{StatusCode: 200}, httptest.NewRequest("GET", "/", nil), nil)
}

func TestResponseInfo(t *testing.T) {
	ri := ResponseInfo{StatusCode: 200, BytesWritten: 42, HeadersSent: true}
	if ri.StatusCode != 200 {
		t.Fatalf("StatusCode = %d", ri.StatusCode)
	}
	if ri.BytesWritten != 42 {
		t.Fatalf("BytesWritten = %d", ri.BytesWritten)
	}
	if !ri.HeadersSent {
		t.Fatal("HeadersSent should be true")
	}
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

	if mw.Name() != "mock" {
		t.Fatalf("Name() = %q", mw.Name())
	}

	handler := mw.Handler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte("hello")) //nolint:errcheck
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/test", nil))

	if !md.preRequestCalled {
		t.Fatal("PreRequest should be called")
	}
	if !md.postRequestCalled {
		t.Fatal("PostRequest should be called")
	}
	if md.postResponseInfo.StatusCode != http.StatusCreated {
		t.Fatalf("StatusCode = %d, want %d", md.postResponseInfo.StatusCode, http.StatusCreated)
	}
	if md.postResponseInfo.BytesWritten != 5 {
		t.Fatalf("BytesWritten = %d, want 5", md.postResponseInfo.BytesWritten)
	}
	if !md.postResponseInfo.HeadersSent {
		t.Fatal("HeadersSent should be true")
	}
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

	if !called {
		t.Fatal("handler should be called with nil driver (NoopDriver fallback)")
	}
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

	if called {
		t.Fatal("handler should not be called when PreRequest fails")
	}
	if !errDriver.postCalled {
		t.Fatal("PostRequest should still be called after PreRequest error")
	}
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

	if rr.statusCode != http.StatusCreated {
		t.Fatalf("statusCode = %d, want %d", rr.statusCode, http.StatusCreated)
	}
}

func TestResponseRecorder_Write(t *testing.T) {
	rec := httptest.NewRecorder()
	rr := &responseRecorder{ResponseWriter: rec, statusCode: http.StatusOK}

	n, err := rr.Write([]byte("hello"))
	if err != nil {
		t.Fatal(err)
	}
	if n != 5 {
		t.Fatalf("n = %d, want 5", n)
	}
	if rr.bytesWritten != 5 {
		t.Fatalf("bytesWritten = %d, want 5", rr.bytesWritten)
	}
	if !rr.headersSent {
		t.Fatal("headersSent should be true after Write")
	}
}

func TestResponseRecorder_Unwrap(t *testing.T) {
	rec := httptest.NewRecorder()
	rr := &responseRecorder{ResponseWriter: rec}
	if rr.Unwrap() != rec {
		t.Fatal("Unwrap should return underlying ResponseWriter")
	}
}
