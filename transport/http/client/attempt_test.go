// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package client_test

import (
	"context"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/transport/http/client/limiters"

	httpclient "github.com/altessa-s/go-atlas/transport/http/client"
)

func TestWithAttemptTimeout(t *testing.T) {
	t.Parallel()

	const timeout = 300 * time.Millisecond
	tests := []struct {
		name  string
		first http.HandlerFunc
	}{
		{"hanging attempt is retried", func(_ http.ResponseWriter, r *http.Request) { <-r.Context().Done() }},
		{"retry-after longer than the timeout", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusServiceUnavailable)
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var hits atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if hits.Add(1) == 1 {
					tc.first(w, r)
					return
				}
				_, _ = io.WriteString(w, "ok")
			}))
			t.Cleanup(srv.Close)

			c := httpclient.New(loopback, httpclient.WithRetryMax(1),
				httpclient.WithRetryWait(time.Millisecond, time.Millisecond), httpclient.WithAttemptTimeout(timeout))
			resp, err := get(t, c, srv.URL)
			require.NoError(t, err)
			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err, "the body is readable after the attempt returned")
			require.Equal(t, "ok", string(body))
			require.Equal(t, int32(2), hits.Load())
		})
	}
}

func TestWithAttemptTimeout_BoundsBody(t *testing.T) {
	t.Parallel()

	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		select { // never finish the body
		case <-release:
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(func() { close(release) })

	c := httpclient.New(loopback, httpclient.WithRetryMax(0), httpclient.WithAttemptTimeout(200*time.Millisecond))
	resp, err := get(t, c, srv.URL)
	require.NoError(t, err)
	start := time.Now()
	_, err = io.ReadAll(resp.Body)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Less(t, time.Since(start), 5*time.Second)
}

func TestWithAttemptTimeout_BoundsLimiterWait(t *testing.T) {
	t.Parallel()

	srv, hits := failingServer(t, 0)
	var calls atomic.Int32
	waiting := limiters.Func(func(ctx context.Context, _ string) error {
		calls.Add(1)
		<-ctx.Done() // a limiter that only gives up with the context
		return ctx.Err()
	})
	c := httpclient.New(loopback, httpclient.WithRetryMax(3), httpclient.WithRetryWait(time.Millisecond, time.Millisecond),
		httpclient.WithLimiter(waiting), httpclient.WithLimiterPerAttempt(), httpclient.WithAttemptTimeout(200*time.Millisecond))

	start := time.Now()
	_, err := get(t, c, srv.URL)
	require.ErrorIs(t, err, limiters.ErrRateLimited)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Less(t, time.Since(start), 5*time.Second)
	require.Equal(t, int32(1), calls.Load(), "a limiter refusal is not retried")
	require.Zero(t, hits.Load())
}

// challengeServer answers 503 with a challenge cookie until a request carries
// it, recording the Cookie headers it receives.
type challengeServer struct {
	mu      sync.Mutex
	cookies []string
}

func (s *challengeServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	s.cookies = append(s.cookies, strings.Join(r.Header.Values("Cookie"), "|"))
	s.mu.Unlock()
	if c, err := r.Cookie("challenge"); err != nil || c.Value != "accepted" {
		http.SetCookie(w, &http.Cookie{Name: "challenge", Value: "accepted", Path: "/"})
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	_, _ = io.WriteString(w, "ok")
}

func (s *challengeServer) seen() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.cookies...)
}

func TestRetryKeepsCookiesOfDiscardedReplies(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct{ name, path string }{
		{"direct", "/to"},
		{"redirected destination", "/from"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			challenge := &challengeServer{}
			mux := http.NewServeMux()
			mux.Handle("/to", challenge)
			mux.HandleFunc("/from", func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/to", http.StatusFound) })
			srv := httptest.NewServer(mux)
			t.Cleanup(srv.Close)

			jar, err := cookiejar.New(nil)
			require.NoError(t, err)
			c := httpclient.New(loopback, httpclient.WithClient(&http.Client{Jar: jar}),
				httpclient.WithRetryMax(2), httpclient.WithRetryWait(time.Millisecond, time.Millisecond))

			req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL+tc.path, nil)
			require.NoError(t, err)
			req.AddCookie(&http.Cookie{Name: "user", Value: "1"})
			resp, err := c.Do(req)
			require.NoError(t, err)
			t.Cleanup(func() { _ = resp.Body.Close() })
			require.Equal(t, http.StatusOK, resp.StatusCode)

			seen := challenge.seen()
			require.Len(t, seen, 2, "one challenge, one retry")
			require.Equal(t, "user=1; challenge=accepted", seen[1],
				"the retry carries the caller's cookie and the challenge cookie once")
		})
	}
}

// cookieRecorder answers the first failures requests with 503 and the
// response headers set, then 200, recording the Cookie header of each request.
type cookieRecorder struct {
	failures int
	setFirst func(w http.ResponseWriter)

	mu      sync.Mutex
	cookies []string
}

func (s *cookieRecorder) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	s.cookies = append(s.cookies, strings.Join(r.Header.Values("Cookie"), "|"))
	n := len(s.cookies)
	s.mu.Unlock()
	if n <= s.failures {
		if s.setFirst != nil {
			s.setFirst(w)
		}
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	_, _ = io.WriteString(w, "ok")
}

func (s *cookieRecorder) seen() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.cookies...)
}

func TestRetryDropsDeletedAndExpiredJarCookies(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		session  *http.Cookie
		setFirst func(w http.ResponseWriter)
		wait     time.Duration
	}{
		{
			name:     "deleted by the discarded reply",
			session:  &http.Cookie{Name: "session", Value: "old", Path: "/"},
			setFirst: func(w http.ResponseWriter) { http.SetCookie(w, &http.Cookie{Name: "session", Path: "/", MaxAge: -1}) },
			wait:     time.Millisecond,
		},
		{
			name:    "expired during the backoff",
			session: &http.Cookie{Name: "session", Value: "old", Path: "/", Expires: time.Now().Add(time.Second)},
			wait:    1500 * time.Millisecond,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rec := &cookieRecorder{failures: 1, setFirst: tc.setFirst}
			srv := httptest.NewServer(rec)
			t.Cleanup(srv.Close)

			jar, err := cookiejar.New(nil)
			require.NoError(t, err)
			u, err := url.Parse(srv.URL)
			require.NoError(t, err)
			jar.SetCookies(u, []*http.Cookie{tc.session})

			c := httpclient.New(loopback, httpclient.WithClient(&http.Client{Jar: jar}),
				httpclient.WithRetryMax(1), httpclient.WithRetryWait(tc.wait, tc.wait), httpclient.WithRetryJitter(0))
			req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL, nil)
			require.NoError(t, err)
			req.AddCookie(&http.Cookie{Name: "user", Value: "1"})
			resp, err := c.Do(req)
			require.NoError(t, err)
			t.Cleanup(func() { _ = resp.Body.Close() })

			seen := rec.seen()
			require.Len(t, seen, 2)
			require.Equal(t, "user=1; session=old", seen[0])
			require.Equal(t, "user=1", seen[1], "the stale jar cookie is not resent; the caller's cookie is")
		})
	}
}

func TestFailedRequestStoresCookies(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		retryMax int
	}{
		{"no retries", 0},
		{"retries exhausted", 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var hits atomic.Int32
			attempts := int32(tc.retryMax + 1)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				n := hits.Add(1)
				if n <= attempts {
					if n == attempts { // the last attempt of the failing request
						http.SetCookie(w, &http.Cookie{Name: "challenge", Value: "accepted", Path: "/"})
					}
					w.WriteHeader(http.StatusServiceUnavailable)
					return
				}
				if c, err := r.Cookie("challenge"); err != nil || c.Value != "accepted" {
					w.WriteHeader(http.StatusForbidden)
					return
				}
				_, _ = io.WriteString(w, "ok")
			}))
			t.Cleanup(srv.Close)

			jar, err := cookiejar.New(nil)
			require.NoError(t, err)
			c := httpclient.New(loopback, httpclient.WithClient(&http.Client{Jar: jar}),
				httpclient.WithRetryMax(tc.retryMax), httpclient.WithRetryWait(time.Millisecond, time.Millisecond))

			_, err = get(t, c, srv.URL)
			require.Error(t, err, "the first request fails")
			resp, err := get(t, c, srv.URL)
			require.NoError(t, err)
			require.Equal(t, http.StatusOK, resp.StatusCode, "the next request carries the cookie of the failed one")
		})
	}
}
