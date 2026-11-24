// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"crypto/x509"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/observability/metrics/metricstest"

	egressrule "github.com/altessa-s/go-atlas-net/egressrule"
	httpclient "github.com/altessa-s/go-atlas-net/http/client"
	netproxy "github.com/altessa-s/go-atlas-net/proxyconfig"
	authconfig "github.com/altessa-s/go-atlas/config/auth"
	httpconfig "github.com/altessa-s/go-atlas/config/http"
	proxyconfig "github.com/altessa-s/go-atlas/config/proxy"
)

func TestRejectsProxyWithSSRFProtection(t *testing.T) {
	t.Parallel()
	proxy := &proxyconfig.Config{Mode: netproxy.ModeURL, URL: "http://proxy.example:8080"}
	_, err := New(&authconfig.OIDC{Proxy: proxy}).buildProviderOptions(t.Context())
	require.ErrorIs(t, err, httpclient.ErrSSRFUnsafeTransport)
}

func TestProviderBuilder_Build_SSRF(t *testing.T) {
	t.Parallel()

	// Discovery requires HTTPS. Run in a separate process so the test TLS
	// root used by the factory's HTTP client is isolated from other tests.
	const childEnv = "ATLAS_TEST_OIDC_SSRF_TLS"
	if os.Getenv(childEnv) == "" {
		cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestProviderBuilder_Build_SSRF$", "-test.v")
		cmd.Env = append(cmd.Environ(), childEnv+"=1", "GODEBUG=x509usefallbackroots=1")
		output, err := cmd.CombinedOutput()
		require.NoError(t, err, "%s", output)
		t.Logf("%s", output)
		return
	}

	const discoveryPath = "/.well-known/openid-configuration"
	var discoveryHits, jwksHits atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc(discoveryPath, func(w http.ResponseWriter, r *http.Request) {
		discoveryHits.Add(1)
		issuer := "https://" + r.Host
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"issuer":%q,"jwks_uri":%q,"id_token_signing_alg_values_supported":["RS256"]}`,
			issuer, issuer+"/jwks")
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, _ *http.Request) {
		jwksHits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"keys":[]}`)
	})
	srv := httptest.NewTLSServer(mux)
	t.Cleanup(srv.Close)
	roots := x509.NewCertPool()
	roots.AddCert(srv.Certificate())
	x509.SetFallbackRoots(roots)

	for _, tc := range []struct {
		name     string
		cfg      *httpconfig.ClientSSRF
		blocked  bool
		wantRule string
	}{
		{name: "absent", blocked: true},
		{name: "allowed CIDR", cfg: &httpconfig.ClientSSRF{AllowedCIDRs: []string{"127.0.0.0/8"}}},
		{name: "disabled", cfg: &httpconfig.ClientSSRF{Disabled: true}},
		{name: "matching deny rule", cfg: &httpconfig.ClientSSRF{
			AllowedCIDRs: []string{"127.0.0.0/8"},
			Deny:         []egressrule.Rule{{Host: "127.0.0.1"}},
		}, blocked: true, wantRule: "deny[0] host=127.0.0.1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			discoveryHits.Store(0)
			jwksHits.Store(0)
			meter := metricstest.NewTestCollector()
			p, err := New(&authconfig.OIDC{DiscoveryUrl: srv.URL + discoveryPath, SSRF: tc.cfg}).UseMeter(meter).Build(t.Context())
			require.EqualValues(t, 1, metricstest.GetHistogramCount(t, meter, "test_auth_oidc_discovery_duration_seconds"))
			if tc.blocked {
				require.ErrorIs(t, err, httpclient.ErrSSRFBlocked)
				require.Equal(t, tc.wantRule, httpclient.IsSSRFError(err).Rule)
				require.Zero(t, discoveryHits.Load())
				require.Zero(t, jwksHits.Load())
				return
			}
			require.NoError(t, err)
			t.Cleanup(p.Close)
			require.EqualValues(t, 1, discoveryHits.Load())
			require.EqualValues(t, 1, jwksHits.Load())
		})
	}
}
