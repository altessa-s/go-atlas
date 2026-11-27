// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package s3probe_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/store/s3probe"

	httpclient "github.com/altessa-s/go-atlas-net/http/client"
)

var loopback = netip.MustParsePrefix("127.0.0.0/8")

func policy(t *testing.T, allow, deny []httpclient.EgressRule) *httpclient.EgressPolicy {
	t.Helper()

	p, err := httpclient.NewEgressPolicy(allow, deny)
	require.NoError(t, err)

	return p
}

func allowLoopback(t *testing.T) *httpclient.EgressPolicy {
	t.Helper()

	return policy(t, []httpclient.EgressRule{{CIDR: loopback}}, nil)
}

func request(endpoint string, trusted bool) s3probe.Request {
	return s3probe.Request{
		Endpoint:  endpoint,
		Bucket:    "reports",
		AccessKey: "AKIATEST",
		SecretKey: "secret-value",
		Trusted:   trusted,
	}
}

// statusServer answers every request with status and counts the requests.
func statusServer(t *testing.T, status int) (*httptest.Server, *atomic.Int32) {
	t.Helper()

	var hits atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)

		if status >= 300 && status < 400 {
			w.Header().Set("Location", "http://127.0.0.1:1/elsewhere")
		}

		w.WriteHeader(status)
	}))
	t.Cleanup(srv.Close)

	return srv, &hits
}

func TestHeadBucketStatuses(t *testing.T) {
	t.Parallel()

	for _, status := range []int{200, 301, 400, 401, 403, 404, 500} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			t.Parallel()

			srv, hits := statusServer(t, status)
			res, err := s3probe.New(allowLoopback(t)).HeadBucket(t.Context(), request(srv.URL, true))

			require.NoError(t, err)
			assert.Equal(t, status, res.StatusCode)
			assert.EqualValues(t, 1, hits.Load(), "redirects and retries must not repeat the request")
		})
	}
}

func TestHeadBucketSignsPathStyleRequest(t *testing.T) {
	t.Parallel()

	var got *http.Request

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Clone(r.Context())
	}))
	t.Cleanup(srv.Close)

	req := request(srv.URL+"/", true)
	_, err := s3probe.New(allowLoopback(t)).HeadBucket(t.Context(), req)
	require.NoError(t, err)

	require.NotNil(t, got)
	assert.Equal(t, http.MethodHead, got.Method)
	assert.Equal(t, "/reports", got.URL.Path)

	auth := got.Header.Get("Authorization")
	assert.True(t, strings.HasPrefix(auth, "AWS4-HMAC-SHA256 Credential=AKIATEST/"), auth)
	assert.Contains(t, auth, "/us-east-1/s3/aws4_request")
	assert.NotContains(t, auth, req.SecretKey)
	assert.NotEmpty(t, got.Header.Get("X-Amz-Date"))
	assert.NotEmpty(t, got.Header.Get("X-Amz-Content-Sha256"))

	req.Region = "eu-west-1"
	_, err = s3probe.New(allowLoopback(t)).HeadBucket(t.Context(), req)
	require.NoError(t, err)
	assert.Contains(t, got.Header.Get("Authorization"), "/eu-west-1/s3/aws4_request")
}

func TestHeadBucketTransportErrors(t *testing.T) {
	t.Parallel()

	t.Run("unreachable", func(t *testing.T) {
		t.Parallel()

		srv := httptest.NewServer(http.NotFoundHandler())
		endpoint := srv.URL
		srv.Close()

		_, err := s3probe.New(allowLoopback(t)).HeadBucket(t.Context(), request(endpoint, true))

		require.ErrorIs(t, err, s3probe.ErrUnreachable)
		assert.NotErrorIs(t, err, s3probe.ErrPolicyRefused)

		te, ok := errors.AsType[*s3probe.TransportError](err)
		require.True(t, ok)
		assert.Equal(t, s3probe.KindUnreachable, te.Kind)
	})

	t.Run("tls", func(t *testing.T) {
		t.Parallel()

		srv := httptest.NewTLSServer(http.NotFoundHandler())
		t.Cleanup(srv.Close)

		_, err := s3probe.New(allowLoopback(t)).HeadBucket(t.Context(), request(srv.URL, true))

		require.ErrorIs(t, err, s3probe.ErrTLS)
	})

	t.Run("timeout", func(t *testing.T) {
		t.Parallel()

		release := make(chan struct{})
		srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			<-release
		}))
		t.Cleanup(func() {
			close(release)
			srv.Close()
		})

		req := request(srv.URL, true)
		req.Timeout = 100 * time.Millisecond

		_, err := s3probe.New(allowLoopback(t)).HeadBucket(t.Context(), req)

		require.ErrorIs(t, err, s3probe.ErrTimeout)
	})
}

func TestHeadBucketEgressPolicy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		policy  func(t *testing.T) *httpclient.EgressPolicy
		trusted bool
		refused bool
	}{
		{name: "saved with allow connects", policy: allowLoopback, trusted: true},
		{name: "draft with the same allow is refused", policy: allowLoopback, refused: true},
		{name: "saved without a rule is refused", policy: func(*testing.T) *httpclient.EgressPolicy { return nil }, trusted: true, refused: true},
		{
			name: "saved with an empty policy is refused",
			policy: func(t *testing.T) *httpclient.EgressPolicy {
				return policy(t, nil, nil)
			},
			trusted: true,
			refused: true,
		},
		{
			name: "deny wins over allow",
			policy: func(t *testing.T) *httpclient.EgressPolicy {
				return policy(t, []httpclient.EgressRule{{CIDR: loopback}}, []httpclient.EgressRule{{CIDR: loopback}})
			},
			trusted: true,
			refused: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			srv, hits := statusServer(t, http.StatusOK)
			res, err := s3probe.New(tc.policy(t)).HeadBucket(t.Context(), request(srv.URL, tc.trusted))

			if !tc.refused {
				require.NoError(t, err)
				assert.Equal(t, http.StatusOK, res.StatusCode)
				assert.EqualValues(t, 1, hits.Load())

				return
			}

			require.ErrorIs(t, err, s3probe.ErrPolicyRefused)
			assert.NotErrorIs(t, err, s3probe.ErrUnreachable)
			assert.Zero(t, hits.Load(), "a refused destination must not be connected to")

			te, ok := errors.AsType[*s3probe.TransportError](err)
			require.True(t, ok)
			assert.Equal(t, s3probe.KindPolicy, te.Kind)
		})
	}
}

func TestHeadBucketInvalidRequest(t *testing.T) {
	t.Parallel()

	tests := map[string]func(*s3probe.Request){
		"no endpoint":      func(r *s3probe.Request) { r.Endpoint = "" },
		"unsupported":      func(r *s3probe.Request) { r.Endpoint = "ftp://host" },
		"credentials":      func(r *s3probe.Request) { r.Endpoint = "http://u:p@host" },
		"no bucket":        func(r *s3probe.Request) { r.Bucket = "" },
		"bucket with path": func(r *s3probe.Request) { r.Bucket = "a/b" },
		"no access key":    func(r *s3probe.Request) { r.AccessKey = "" },
		"no secret key":    func(r *s3probe.Request) { r.SecretKey = "" },
	}

	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			req := request("http://127.0.0.1:1", true)
			mutate(&req)

			_, err := s3probe.New(nil).HeadBucket(t.Context(), req)

			require.ErrorIs(t, err, s3probe.ErrInvalidRequest)
			assert.NotContains(t, err.Error(), "secret-value")
		})
	}
}

func TestKindString(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "policy", s3probe.KindPolicy.String())
	assert.Equal(t, "tls", s3probe.KindTLS.String())
	assert.Equal(t, "timeout", s3probe.KindTimeout.String())
	assert.Equal(t, "unreachable", s3probe.KindUnreachable.String())
	assert.Equal(t, "unknown", s3probe.Kind(99).String())
}
