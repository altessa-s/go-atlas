// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/altessa-s/go-atlas/internal/testhelpers"
)

func TestNew_ReturnsClient(t *testing.T) {
	c := New()
	if c == nil {
		t.Fatal("New() returned nil")
	}
}

func TestNew_WithOptions(t *testing.T) {
	c := New(WithRetryMax(1), WithRetryWait(100*time.Millisecond, 500*time.Millisecond))
	if c == nil {
		t.Fatal("New() returned nil")
	}
}

func TestNewHTTPClient(t *testing.T) {
	c := NewHTTPClient()
	if c == nil {
		t.Fatal("NewHTTPClient() returned nil")
	}
}

func TestHTTPClient_Get(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewHTTPClient(WithRetryMax(0))
	resp, err := c.Get(t.Context(), srv.URL)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
}

func TestHTTPClient_PostJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Content-Type") != "application/json" {
			t.Fatalf("Content-Type = %q", r.Header.Get("Content-Type"))
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	c := NewHTTPClient(WithRetryMax(0))
	resp, err := c.PostJSON(t.Context(), srv.URL, map[string]string{"key": "value"})
	if err != nil {
		t.Fatalf("PostJSON() error = %v", err)
	}
	defer resp.Body.Close()
}

func TestHTTPClient_Delete(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Fatalf("method = %q", r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c := NewHTTPClient(WithRetryMax(0))
	resp, err := c.Delete(t.Context(), srv.URL)
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	defer resp.Body.Close()
}

func TestHTTPClient_Head(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodHead {
			t.Fatalf("method = %q", r.Method)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewHTTPClient(WithRetryMax(0))
	resp, err := c.Head(t.Context(), srv.URL)
	if err != nil {
		t.Fatalf("Head() error = %v", err)
	}
	defer resp.Body.Close()
}

func TestHTTPClient_GetWithOptions(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Custom") != "test" {
			t.Fatalf("X-Custom = %q", r.Header.Get("X-Custom"))
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewHTTPClient(WithRetryMax(0))
	resp, err := c.GetWithOptions(t.Context(), srv.URL, WithHeader("X-Custom", "test"))
	if err != nil {
		t.Fatalf("GetWithOptions() error = %v", err)
	}
	defer resp.Body.Close()
}

func TestWithBearerToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer my-token" {
			t.Fatalf("Authorization = %q", r.Header.Get("Authorization"))
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewHTTPClient(WithRetryMax(0))
	resp, err := c.GetWithOptions(t.Context(), srv.URL, WithBearerToken("my-token"))
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	defer resp.Body.Close()
}

func TestWithBasicAuth(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, p, ok := r.BasicAuth()
		if !ok || u != "user" || p != "pass" {
			t.Fatalf("BasicAuth = %q %q %v", u, p, ok)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewHTTPClient(WithRetryMax(0))
	resp, err := c.GetWithOptions(t.Context(), srv.URL, WithBasicAuth("user", "pass"))
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	defer resp.Body.Close()
}

func TestWithRequestTimeout(t *testing.T) {
	// Verify that WithRequestTimeout sets a context with deadline
	req, _ := http.NewRequest("GET", "http://example.com", nil)
	WithRequestTimeout(5 * time.Second)(req)
	deadline, ok := req.Context().Deadline()
	if !ok {
		t.Fatal("no deadline set on request context")
	}
	if time.Until(deadline) > 6*time.Second {
		t.Fatal("deadline too far in the future")
	}
}

func TestWithRequestDeadline(t *testing.T) {
	req, _ := http.NewRequest("GET", "http://example.com", nil)
	dl := time.Now().Add(10 * time.Second)
	WithRequestDeadline(dl)(req)
	gotDeadline, ok := req.Context().Deadline()
	if !ok {
		t.Fatal("no deadline set")
	}
	if gotDeadline.Sub(dl) > time.Second {
		t.Fatal("deadline mismatch")
	}
}

func TestIsUnexpectedStatusError(t *testing.T) {
	statusErr := &UnexpectedStatusError{Status: 404}
	if IsUnexpectedStatusError(statusErr) == nil {
		t.Fatal("should find UnexpectedStatusError")
	}
	if IsUnexpectedStatusError(context.Canceled) != nil {
		t.Fatal("should return nil for non-status error")
	}
}

func TestIsCircuitBreakerOpen(t *testing.T) {
	if IsCircuitBreakerOpen(context.Canceled) {
		t.Fatal("should be false for non-CB error")
	}
}

func TestRoundTripFunc(t *testing.T) {
	fn := testhelpers.RoundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK}, nil
	})
	req, _ := http.NewRequest("GET", "http://test", nil)
	resp, err := fn.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip() error = %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
}
