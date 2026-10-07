// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package client

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"
)

func TestNew_ReturnsClient(t *testing.T) {
	c := New()
	require.NotNil(t, c)
}

func TestNew_WithOptions(t *testing.T) {
	c := New(WithRetryMax(1), WithRetryWait(100*time.Millisecond, 500*time.Millisecond))
	require.NotNil(t, c)
}

func TestNewHTTPClient(t *testing.T) {
	c := NewHTTPClient()
	require.NotNil(t, c)
}

func TestHTTPClient_Get(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewHTTPClient(WithRetryMax(0))
	resp, err := c.Get(t.Context(), srv.URL)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestHTTPClient_PostJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "application/json", r.Header.Get("Content-Type"))
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	c := NewHTTPClient(WithRetryMax(0))
	resp, err := c.PostJSON(t.Context(), srv.URL, map[string]string{"key": "value"})
	require.NoError(t, err)
	defer resp.Body.Close()
}

func TestHTTPClient_Delete(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodDelete, r.Method)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c := NewHTTPClient(WithRetryMax(0))
	resp, err := c.Delete(t.Context(), srv.URL)
	require.NoError(t, err)
	defer resp.Body.Close()
}

func TestHTTPClient_Head(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodHead, r.Method)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewHTTPClient(WithRetryMax(0))
	resp, err := c.Head(t.Context(), srv.URL)
	require.NoError(t, err)
	defer resp.Body.Close()
}

func TestHTTPClient_GetWithOptions(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "test", r.Header.Get("X-Custom"))
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewHTTPClient(WithRetryMax(0))
	resp, err := c.GetWithOptions(t.Context(), srv.URL, WithHeader("X-Custom", "test"))
	require.NoError(t, err)
	defer resp.Body.Close()
}

func TestWithBearerToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "Bearer my-token", r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewHTTPClient(WithRetryMax(0))
	resp, err := c.GetWithOptions(t.Context(), srv.URL, WithBearerToken("my-token"))
	require.NoError(t, err)
	defer resp.Body.Close()
}

func TestWithBasicAuth(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, p, ok := r.BasicAuth()
		require.True(t, ok, "BasicAuth should be present")
		require.Equal(t, "user", u)
		require.Equal(t, "pass", p)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewHTTPClient(WithRetryMax(0))
	resp, err := c.GetWithOptions(t.Context(), srv.URL, WithBasicAuth("user", "pass"))
	require.NoError(t, err)
	defer resp.Body.Close()
}

func TestWithRequestTimeout(t *testing.T) {
	// Verify that WithRequestTimeout sets a context with deadline
	req, _ := http.NewRequest("GET", "http://example.com", nil)
	WithRequestTimeout(5 * time.Second)(req)
	deadline, ok := req.Context().Deadline()
	require.True(t, ok, "no deadline set on request context")
	require.True(t, time.Until(deadline) <= 6*time.Second, "deadline too far in the future")
}

func TestWithRequestDeadline(t *testing.T) {
	req, _ := http.NewRequest("GET", "http://example.com", nil)
	dl := time.Now().Add(10 * time.Second)
	WithRequestDeadline(dl)(req)
	gotDeadline, ok := req.Context().Deadline()
	require.True(t, ok, "no deadline set")
	require.True(t, gotDeadline.Sub(dl) <= time.Second, "deadline mismatch")
}

func TestIsUnexpectedStatusError(t *testing.T) {
	statusErr := &UnexpectedStatusError{Status: 404}
	require.NotNil(t, IsUnexpectedStatusError(statusErr))
	require.Nil(t, IsUnexpectedStatusError(context.Canceled))
}

func TestIsCircuitBreakerOpen(t *testing.T) {
	require.False(t, IsCircuitBreakerOpen(context.Canceled), "should be false for non-CB error")
	require.True(t, IsCircuitBreakerOpen(fmt.Errorf("wrapped: %w", &CircuitBreakerError{Name: "x", State: "open"})))
}

// rwBody is a writable response body, as returned for 101 Switching Protocols.
type rwBody struct {
	io.Reader
	written []byte
	closed  bool
}

func (b *rwBody) Write(p []byte) (int, error) {
	b.written = append(b.written, p...)
	return len(p), nil
}

func (b *rwBody) Close() error {
	b.closed = true
	return nil
}

func TestAttachCancel(t *testing.T) {
	t.Parallel()

	t.Run("cancels on Close", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(t.Context())
		resp := attachCancel(&http.Response{Body: io.NopCloser(strings.NewReader("x"))}, cancel)
		require.NoError(t, ctx.Err(), "must not cancel before Close")
		require.NoError(t, resp.Body.Close())
		require.ErrorIs(t, ctx.Err(), context.Canceled)
	})

	t.Run("keeps Write", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(t.Context())
		orig := &rwBody{Reader: strings.NewReader("")}
		resp := attachCancel(&http.Response{Body: orig}, cancel)
		w, ok := resp.Body.(io.Writer)
		require.True(t, ok, "writable body must stay writable")
		_, err := w.Write([]byte("ping"))
		require.NoError(t, err)
		require.Equal(t, "ping", string(orig.written))
		require.NoError(t, resp.Body.Close())
		require.True(t, orig.closed)
		require.ErrorIs(t, ctx.Err(), context.Canceled)
	})

	t.Run("nil cancel leaves body", func(t *testing.T) {
		t.Parallel()

		body := io.NopCloser(strings.NewReader("x"))
		resp := attachCancel(&http.Response{Body: body}, nil)
		require.Equal(t, body, resp.Body)
	})
}

func TestRoundTripFunc(t *testing.T) {
	fn := testhelpers.RoundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK}, nil
	})
	req, _ := http.NewRequest("GET", "http://test", nil)
	resp, err := fn.RoundTrip(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
}
