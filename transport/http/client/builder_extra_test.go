// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package client_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	client "github.com/altessa-s/go-atlas/transport/http/client"
)

func newTestClient(srv *httptest.Server) client.HTTPClient {
	return client.NewHTTPClient(client.WithClient(srv.Client()), client.WithRetryMax(0))
}

func TestRequestBuilder_GET(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
	}))
	defer srv.Close()

	rb := client.NewRequestBuilder(newTestClient(srv))
	resp, err := rb.GET(srv.URL + "/test").Send(t.Context())
	require.NoError(t, err)
	resp.Body.Close()
}

func TestRequestBuilder_POST_JSONBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	rb := client.NewRequestBuilder(newTestClient(srv))
	resp, err := rb.POST(srv.URL + "/test").JSONBody(map[string]string{"key": "val"}).Send(t.Context())
	require.NoError(t, err)
	resp.Body.Close()
}

func TestRequestBuilder_PUT(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("method = %s, want PUT", r.Method)
		}
	}))
	defer srv.Close()

	rb := client.NewRequestBuilder(newTestClient(srv))
	resp, err := rb.PUT(srv.URL + "/test").StringBody("data").Send(t.Context())
	require.NoError(t, err)
	resp.Body.Close()
}

func TestRequestBuilder_PATCH(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Errorf("method = %s, want PATCH", r.Method)
		}
	}))
	defer srv.Close()

	rb := client.NewRequestBuilder(newTestClient(srv))
	resp, err := rb.PATCH(srv.URL + "/test").BytesBody([]byte("patch")).Send(t.Context())
	require.NoError(t, err)
	resp.Body.Close()
}

func TestRequestBuilder_DELETE(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("method = %s, want DELETE", r.Method)
		}
	}))
	defer srv.Close()

	rb := client.NewRequestBuilder(newTestClient(srv))
	resp, err := rb.DELETE(srv.URL + "/test").Send(t.Context())
	require.NoError(t, err)
	resp.Body.Close()
}

func TestRequestBuilder_HEAD(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodHead {
			t.Errorf("method = %s, want HEAD", r.Method)
		}
	}))
	defer srv.Close()

	rb := client.NewRequestBuilder(newTestClient(srv))
	resp, err := rb.HEAD(srv.URL + "/test").Send(t.Context())
	require.NoError(t, err)
	resp.Body.Close()
}

func TestRequestBuilder_OPTIONS(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodOptions {
			t.Errorf("method = %s, want OPTIONS", r.Method)
		}
	}))
	defer srv.Close()

	rb := client.NewRequestBuilder(newTestClient(srv))
	resp, err := rb.OPTIONS(srv.URL + "/test").Send(t.Context())
	require.NoError(t, err)
	resp.Body.Close()
}

func TestRequestBuilder_CustomMethod(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "CUSTOM" {
			t.Errorf("method = %s, want CUSTOM", r.Method)
		}
	}))
	defer srv.Close()

	rb := client.NewRequestBuilder(newTestClient(srv))
	resp, err := rb.Method("CUSTOM", srv.URL+"/test").Send(t.Context())
	require.NoError(t, err)
	resp.Body.Close()
}

func TestRequestBuilder_Headers(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Custom") != "value" {
			t.Errorf("X-Custom = %q", r.Header.Get("X-Custom"))
		}
	}))
	defer srv.Close()

	rb := client.NewRequestBuilder(newTestClient(srv))
	resp, err := rb.GET(srv.URL+"/test").
		Header("X-Custom", "value").
		Accept("text/plain").
		ContentType("application/json").
		UserAgent("test-agent").
		Send(t.Context())
	require.NoError(t, err)
	resp.Body.Close()
}

func TestRequestBuilder_QueryParams(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("key") != "val" {
			t.Errorf("query key = %q", r.URL.Query().Get("key"))
		}
	}))
	defer srv.Close()

	rb := client.NewRequestBuilder(newTestClient(srv))
	resp, err := rb.GET(srv.URL+"/test").QueryParam("key", "val").Send(t.Context())
	require.NoError(t, err)
	resp.Body.Close()
}

func TestRequestBuilder_BearerToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			t.Error("expected Authorization header")
		}
	}))
	defer srv.Close()

	rb := client.NewRequestBuilder(newTestClient(srv))
	resp, err := rb.GET(srv.URL + "/test").BearerToken("tok123").Send(t.Context())
	require.NoError(t, err)
	resp.Body.Close()
}

func TestRequestBuilder_BasicAuth(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, p, ok := r.BasicAuth()
		if !ok || u != "user" || p != "pass" {
			t.Errorf("BasicAuth = %q %q %v", u, p, ok)
		}
	}))
	defer srv.Close()

	rb := client.NewRequestBuilder(newTestClient(srv))
	resp, err := rb.GET(srv.URL+"/test").BasicAuth("user", "pass").Send(t.Context())
	require.NoError(t, err)
	resp.Body.Close()
}

func TestRequestBuilder_APIKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-Key") == "" {
			t.Error("expected X-API-Key header")
		}
	}))
	defer srv.Close()

	rb := client.NewRequestBuilder(newTestClient(srv))
	resp, err := rb.GET(srv.URL+"/test").APIKey("X-API-Key", "secret").Send(t.Context())
	require.NoError(t, err)
	resp.Body.Close()
}

func TestRequestBuilder_Timeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()

	rb := client.NewRequestBuilder(newTestClient(srv))
	resp, err := rb.GET(srv.URL + "/test").Timeout(5 * time.Second).Send(t.Context())
	require.NoError(t, err)
	resp.Body.Close()
}

func TestRequestBuilder_Body(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()

	rb := client.NewRequestBuilder(newTestClient(srv))
	resp, err := rb.POST(srv.URL + "/test").Body(strings.NewReader("raw")).Send(t.Context())
	require.NoError(t, err)
	resp.Body.Close()
}

func TestRequestBuilder_Build_NoURL(t *testing.T) {
	c := client.NewHTTPClient(client.WithRetryMax(0))
	rb := client.NewRequestBuilder(c)
	_, err := rb.GET("").Build()
	require.Error(t, err, "Build with empty URL should fail")
}

// newStreamingServer flushes "head;" and writes "tail" only once release is
// closed, so the body is still streaming when Do returns.
func newStreamingServer(t *testing.T, release <-chan struct{}) *httptest.Server {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "head;")
		_ = http.NewResponseController(w).Flush()
		select {
		case <-release:
			_, _ = io.WriteString(w, "tail")
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestRequestTimeout_StreamingBodyReadable(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		do   func(ctx context.Context, c client.HTTPClient, url string) (*http.Response, error)
	}{
		{
			name: "WithRequestTimeout",
			do: func(ctx context.Context, c client.HTTPClient, url string) (*http.Response, error) {
				return c.GetWithOptions(ctx, url, client.WithRequestTimeout(time.Minute))
			},
		},
		{
			name: "WithRequestDeadline",
			do: func(ctx context.Context, c client.HTTPClient, url string) (*http.Response, error) {
				return c.GetWithOptions(ctx, url, client.WithRequestDeadline(time.Now().Add(time.Minute)))
			},
		},
		{
			name: "builder Timeout",
			do: func(ctx context.Context, c client.HTTPClient, url string) (*http.Response, error) {
				return c.NewRequest().GET(url).Timeout(time.Minute).Send(ctx)
			},
		},
		{
			name: "builder Deadline",
			do: func(ctx context.Context, c client.HTTPClient, url string) (*http.Response, error) {
				return c.NewRequest().GET(url).Deadline(time.Now().Add(time.Minute)).Send(ctx)
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			release := make(chan struct{})
			srv := newStreamingServer(t, release)

			resp, err := tc.do(t.Context(), newTestClient(srv), srv.URL)
			require.NoError(t, err)
			defer resp.Body.Close()

			close(release)
			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err, "per-request timeout must not cancel the body before Close")
			require.Equal(t, "head;tail", string(body))
		})
	}
}

func TestRequestBuilder_SendWithoutContext(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()

	rb := client.NewRequestBuilder(newTestClient(srv))
	resp, err := rb.GET(srv.URL + "/test").SendWithoutContext()
	require.NoError(t, err)
	resp.Body.Close()
}

func TestRequestBuilder_Context(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()

	rb := client.NewRequestBuilder(newTestClient(srv))
	resp, err := rb.GET(srv.URL + "/test").Context(t.Context()).Send(t.Context())
	require.NoError(t, err)
	resp.Body.Close()
}
