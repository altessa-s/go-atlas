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

	"github.com/altessa-s/go-atlas/internal/testhelpers"

	httpclient "github.com/altessa-s/go-atlas/transport/http/client"
)

// countingJar counts SetCookies calls on top of a real jar.
type countingJar struct {
	http.CookieJar
	sets atomic.Int32
}

func (j *countingJar) SetCookies(u *url.URL, cookies []*http.Cookie) {
	j.sets.Add(1)
	j.CookieJar.SetCookies(u, cookies)
}

func newCountingJar(t *testing.T) *countingJar {
	t.Helper()
	jar, err := cookiejar.New(nil)
	require.NoError(t, err)
	return &countingJar{CookieJar: jar}
}

// cookieValue returns the value of name in jar for u, or "".
func cookieValue(jar http.CookieJar, u *url.URL, name string) string {
	for _, c := range jar.Cookies(u) {
		if c.Name == name {
			return c.Value
		}
	}
	return ""
}

func TestWithCookieJar_ClientHasNoJar(t *testing.T) {
	t.Parallel()

	jar := newCountingJar(t)
	c := httpclient.New(loopback, httpclient.WithCookieJar(jar))
	require.Nil(t, c.Jar, "the retry layer owns the jar")

	viaClient := httpclient.New(loopback, httpclient.WithClient(&http.Client{Jar: jar}))
	require.Nil(t, viaClient.Jar, "a WithClient jar is owned by the retry layer too")
}

// TestRetryReadsTheJarAtAttemptTime: another request changes or deletes the
// shared cookie between the first attempt and the retry; the retry sends the
// jar's current state, never the stale value, never twice.
func TestRetryReadsTheJarAtAttemptTime(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name   string
		mutate func(jar http.CookieJar, u *url.URL)
		want   string
	}{
		{"replaced", func(jar http.CookieJar, u *url.URL) {
			jar.SetCookies(u, []*http.Cookie{{Name: "session", Value: "new", Path: "/"}})
		}, "user=1; session=new"},
		{"deleted", func(jar http.CookieJar, u *url.URL) {
			jar.SetCookies(u, []*http.Cookie{{Name: "session", Path: "/", MaxAge: -1}})
		}, "user=1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			jar := newCountingJar(t)
			var (
				mu   sync.Mutex
				seen []string
				u    *url.URL
			)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				seen = append(seen, strings.Join(r.Header.Values("Cookie"), "|"))
				n := len(seen)
				mu.Unlock()
				if n == 1 {
					tc.mutate(jar, u) // a concurrent request updates the shared jar
					w.WriteHeader(http.StatusServiceUnavailable)
					return
				}
				_, _ = io.WriteString(w, "ok")
			}))
			t.Cleanup(srv.Close)
			var err error
			u, err = url.Parse(srv.URL)
			require.NoError(t, err)
			jar.SetCookies(u, []*http.Cookie{{Name: "session", Value: "old", Path: "/"}})

			c := httpclient.New(loopback, httpclient.WithCookieJar(jar),
				httpclient.WithRetryMax(1), httpclient.WithRetryWait(time.Millisecond, time.Millisecond))
			req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL, nil)
			require.NoError(t, err)
			req.AddCookie(&http.Cookie{Name: "user", Value: "1"})
			resp, err := c.Do(req)
			require.NoError(t, err)
			t.Cleanup(func() { _ = resp.Body.Close() })

			mu.Lock()
			defer mu.Unlock()
			require.Equal(t, []string{"user=1; session=old", tc.want}, seen)
		})
	}
}

// TestCancelDuringBackoffDoesNotRestoreCookies: the discarded reply's cookie
// is stored once, on receipt; when the backoff is cut short after another
// request replaced the cookie, it is not stored again over the new value.
func TestCancelDuringBackoffDoesNotRestoreCookies(t *testing.T) {
	t.Parallel()

	jar := newCountingJar(t)
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		http.SetCookie(w, &http.Cookie{Name: "session", Value: "old", Path: "/", MaxAge: 3600})
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(srv.Close)
	u, err := url.Parse(srv.URL)
	require.NoError(t, err)

	c := httpclient.New(loopback, httpclient.WithCookieJar(jar),
		httpclient.WithRetryMax(3), httpclient.WithRetryWait(time.Hour, time.Hour))
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		req, rerr := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
		if rerr != nil {
			done <- rerr
			return
		}
		resp, derr := c.Do(req)
		if resp != nil {
			_ = resp.Body.Close()
		}
		done <- derr
	}()

	// Request A got its 503 and stored the cookie; it now sleeps an hour.
	testhelpers.WaitFor(t, 5*time.Second, func() bool { return cookieValue(jar, u, "session") == "old" }, "cookie stored")
	jar.SetCookies(u, []*http.Cookie{{Name: "session", Value: "new", Path: "/"}}) // request B
	cancel()
	require.ErrorIs(t, <-done, context.Canceled)

	require.Equal(t, "new", cookieValue(jar, u, "session"), "the old value is not restored")
	require.Equal(t, int32(1), hits.Load())
	require.Equal(t, int32(2), jar.sets.Load(), "one store for A's single reply, one for B")
}

// TestRedirectHopCookies: a cookie set by a redirect reply is stored and sent
// to the next hop, and a challenge cookie set by a retried hop reaches its
// retry.
func TestRedirectHopCookies(t *testing.T) {
	t.Parallel()

	var (
		mu   sync.Mutex
		seen []string
	)
	mux := http.NewServeMux()
	mux.HandleFunc("/from", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "hop", Value: "1", Path: "/"})
		http.Redirect(w, r, "/to", http.StatusFound)
	})
	mux.HandleFunc("/to", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = append(seen, strings.Join(r.Header.Values("Cookie"), "|"))
		mu.Unlock()
		if _, err := r.Cookie("challenge"); err != nil {
			http.SetCookie(w, &http.Cookie{Name: "challenge", Value: "accepted", Path: "/"})
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = io.WriteString(w, "ok")
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	jar := newCountingJar(t)
	c := httpclient.New(loopback, httpclient.WithCookieJar(jar),
		httpclient.WithRetryMax(1), httpclient.WithRetryWait(time.Millisecond, time.Millisecond))
	resp, err := get(t, c, srv.URL+"/from")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, []string{"hop=1", "hop=1; challenge=accepted"}, seen)
	require.Equal(t, int32(2), jar.sets.Load(), "each reply with cookies stored once: the 302 and the 503")
}

// TestCookiesFollowTheHostHeader: like http.Client, cookies are looked up and
// stored under req.Host when it is set (a virtual host behind one address).
func TestCookiesFollowTheHostHeader(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		opts []httpclient.Option
	}{
		{"with retries", []httpclient.Option{httpclient.WithRetryMax(1)}},
		{"without retry", []httpclient.Option{httpclient.WithoutRetry()}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var (
				mu     sync.Mutex
				header string
			)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				header = strings.Join(r.Header.Values("Cookie"), "|")
				mu.Unlock()
				http.SetCookie(w, &http.Cookie{Name: "tenant", Value: "t", Path: "/"})
				_, _ = io.WriteString(w, "ok")
			}))
			t.Cleanup(srv.Close)
			gateway, err := url.Parse(srv.URL)
			require.NoError(t, err)
			tenant := &url.URL{Scheme: "http", Host: "tenant.example", Path: "/"}

			jar := newCountingJar(t)
			jar.SetCookies(gateway, []*http.Cookie{{Name: "gw", Value: "1", Path: "/"}})
			c := httpclient.New(append([]httpclient.Option{loopback, httpclient.WithCookieJar(jar)}, tc.opts...)...)

			req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL, nil)
			require.NoError(t, err)
			req.Host = "tenant.example"
			resp, err := c.Do(req)
			require.NoError(t, err)
			t.Cleanup(func() { _ = resp.Body.Close() })

			mu.Lock()
			defer mu.Unlock()
			require.Empty(t, header, "the gateway's cookies are not sent to the tenant virtual host")
			require.Equal(t, "t", cookieValue(jar, tenant, "tenant"), "the tenant's cookie is stored under the tenant host")
			require.Empty(t, cookieValue(jar, gateway, "tenant"), "and not under the gateway")
		})
	}
}

// TestRedirectDropsInitialCookiesTheChainSet: like http.Client with a jar, an
// initial cookie that a redirect reply replaces or deletes is not forwarded
// to the next hops (the jar's value is sent instead, if any).
func TestRedirectDropsInitialCookiesTheChainSet(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		set  *http.Cookie
		want string
	}{
		{"replaced", &http.Cookie{Name: "session", Value: "new", Path: "/"}, "user=1; session=new"},
		{"deleted", &http.Cookie{Name: "session", Path: "/", MaxAge: -1}, "user=1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var (
				mu   sync.Mutex
				seen []string
			)
			mux := http.NewServeMux()
			mux.HandleFunc("/from", func(w http.ResponseWriter, r *http.Request) {
				http.SetCookie(w, tc.set)
				http.Redirect(w, r, "/mid", http.StatusFound)
			})
			mux.HandleFunc("/mid", func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				seen = append(seen, strings.Join(r.Header.Values("Cookie"), "|"))
				mu.Unlock()
				http.Redirect(w, r, "/to", http.StatusFound) // sets nothing: the drop persists along the chain
			})
			mux.HandleFunc("/to", func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				seen = append(seen, strings.Join(r.Header.Values("Cookie"), "|"))
				mu.Unlock()
				_, _ = io.WriteString(w, "ok")
			})
			srv := httptest.NewServer(mux)
			t.Cleanup(srv.Close)

			c := httpclient.New(loopback, httpclient.WithCookieJar(newCountingJar(t)))
			req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL+"/from", nil)
			require.NoError(t, err)
			req.Header.Set("Cookie", "session=old; user=1")
			resp, err := c.Do(req)
			require.NoError(t, err)
			t.Cleanup(func() { _ = resp.Body.Close() })

			mu.Lock()
			defer mu.Unlock()
			require.Equal(t, []string{tc.want, tc.want}, seen, "no stale session on any later hop")
		})
	}
}

// TestCustomCheckRedirectHonored: a caller's CheckRedirect still decides, and
// the redirect reply's cookies are stored.
func TestCustomCheckRedirectHonored(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("/from", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "hop", Value: "1", Path: "/"})
		http.Redirect(w, r, "/to", http.StatusFound)
	})
	var followed atomic.Int32
	mux.HandleFunc("/to", func(w http.ResponseWriter, _ *http.Request) { followed.Add(1) })
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	u, err := url.Parse(srv.URL)
	require.NoError(t, err)

	var checked atomic.Int32
	jar := newCountingJar(t)
	c := httpclient.New(loopback, httpclient.WithCookieJar(jar), httpclient.WithClient(&http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error {
			checked.Add(1)
			return http.ErrUseLastResponse
		},
	}))
	resp, err := get(t, c, srv.URL+"/from")
	require.NoError(t, err)
	require.Equal(t, http.StatusFound, resp.StatusCode)
	require.Equal(t, int32(1), checked.Load())
	require.Zero(t, followed.Load())
	require.Equal(t, "1", cookieValue(jar, u, "hop"))
}

// TestCheckRedirectCookiesKept: the initial cookies are filtered before the
// caller's CheckRedirect runs, so a cookie it sets on the next request is
// sent, also when the initial request had none.
func TestCheckRedirectCookiesKept(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		initial string
		check   func(req *http.Request, _ []*http.Request) error
		want    string
	}{
		{
			name:    "replacement for a cookie the redirect deleted",
			initial: "session=old",
			check: func(req *http.Request, _ []*http.Request) error {
				req.Header.Set("Cookie", "session=replacement")
				return nil
			},
			want: "session=replacement",
		},
		{
			name: "cookie added without initial cookies",
			check: func(req *http.Request, _ []*http.Request) error {
				req.AddCookie(&http.Cookie{Name: "added", Value: "1"})
				return nil
			},
			want: "added=1",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var (
				mu   sync.Mutex
				seen string
			)
			mux := http.NewServeMux()
			mux.HandleFunc("/from", func(w http.ResponseWriter, r *http.Request) {
				http.SetCookie(w, &http.Cookie{Name: "session", Path: "/", MaxAge: -1})
				http.Redirect(w, r, "/to", http.StatusFound)
			})
			mux.HandleFunc("/to", func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				seen = strings.Join(r.Header.Values("Cookie"), "|")
				mu.Unlock()
				_, _ = io.WriteString(w, "ok")
			})
			srv := httptest.NewServer(mux)
			t.Cleanup(srv.Close)

			c := httpclient.New(loopback, httpclient.WithCookieJar(newCountingJar(t)),
				httpclient.WithClient(&http.Client{CheckRedirect: tc.check}))
			req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL+"/from", nil)
			require.NoError(t, err)
			if tc.initial != "" {
				req.Header.Set("Cookie", tc.initial)
			}
			resp, err := c.Do(req)
			require.NoError(t, err)
			t.Cleanup(func() { _ = resp.Body.Close() })

			mu.Lock()
			defer mu.Unlock()
			require.Equal(t, tc.want, seen)
		})
	}
}

// TestDefaultRedirectLimitWithJar: without a CheckRedirect, net/http's limit
// of 10 redirects still applies when the retry layer owns the jar.
func TestDefaultRedirectLimitWithJar(t *testing.T) {
	t.Parallel()

	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		http.Redirect(w, r, "/loop", http.StatusFound)
	}))
	t.Cleanup(srv.Close)

	c := httpclient.New(loopback, httpclient.WithCookieJar(newCountingJar(t)), httpclient.WithRetryMax(0))
	_, err := get(t, c, srv.URL+"/loop")
	// net/http's default policy error is an unexported plain error: its text
	// is the only way to recognize it.
	require.ErrorContains(t, err, "stopped after 10 redirects")
	require.Equal(t, int32(10), hits.Load(), "the 10th redirect reply is the last request sent")
}
