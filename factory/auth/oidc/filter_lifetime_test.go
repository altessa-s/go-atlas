// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/oidc"

	httpclient "github.com/altessa-s/go-atlas-net/http/client"
	netproxy "github.com/altessa-s/go-atlas-net/proxyconfig"
	bloomstorages "github.com/altessa-s/go-atlas/cache/probfilter/bloom/storages"
	bloommemory "github.com/altessa-s/go-atlas/cache/probfilter/bloom/storages/memory"
	authconfig "github.com/altessa-s/go-atlas/config/auth"
	probfilterconfig "github.com/altessa-s/go-atlas/config/probfilter"
	proxyconfig "github.com/altessa-s/go-atlas/config/proxy"
	probfilterfactory "github.com/altessa-s/go-atlas/factory/cache/probfilter"
)

type closeCountingStorage struct {
	*bloommemory.Storage
	closes atomic.Int32
}

func (s *closeCountingStorage) Close(ctx context.Context) error {
	s.closes.Add(1)
	return s.Storage.Close(ctx)
}

func TestFactoryFilterLifetime(t *testing.T) {
	t.Parallel()
	for _, phase := range []string{"option error", "discovery error", "initial sync error", "provider close"} {
		t.Run(phase, func(t *testing.T) {
			t.Parallel()
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Path == "/jwks" {
					_, _ = fmt.Fprint(w, `{"keys":[]}`)
					return
				}
				issuer := "http://" + r.Host
				_, _ = fmt.Fprintf(w, `{"issuer":%q,"jwks_uri":%q,"id_token_signing_alg_values_supported":["RS256"]}`, issuer, issuer+"/jwks")
			}))
			defer srv.Close()
			rev := memoryFilterRevocation()
			rev.Filter.Bloom.Storage.Type = probfilterconfig.StorageTypeRedis
			if phase == "initial sync error" {
				rev.Source = &authconfig.OIDCRevocationSource{File: filepath.Join(t.TempDir(), "missing")}
			}
			cfg := &authconfig.OIDC{DiscoveryUrl: srv.URL, Revocation: rev}
			counts := &closeCountingStorage{Storage: bloommemory.New()}
			b := New(cfg).UseRevocationBloomStorage(probfilterconfig.StorageTypeRedis, func(context.Context, probfilterfactory.BloomStorageSpec) (bloomstorages.Storage, error) {
				return counts, nil
			})
			switch phase {
			case "option error":
				cfg.Proxy = &proxyconfig.Config{Mode: netproxy.ModeURL, URL: "http://proxy.example"}
				_, err := b.Build(t.Context())
				require.ErrorIs(t, err, httpclient.ErrSSRFUnsafeTransport)
			case "discovery error":
				cfg.DiscoveryUrl = "://invalid"
				_, err := b.Build(t.Context())
				require.Error(t, err)
			default:
				opts, err := b.buildProviderOptions(t.Context())
				require.NoError(t, err)
				opts = append(opts, oidc.WithHTTPClient(srv.Client()), oidc.WithDiscoveryAllowLoopbackHTTP())
				p, err := oidc.NewProvider(t.Context(), cfg.DiscoveryUrl, opts...)
				if phase == "initial sync error" {
					require.ErrorIs(t, err, oidc.ErrRevocationCheck)
				} else {
					require.NoError(t, err)
					require.Zero(t, counts.closes.Load())
					p.Close()
					p.Close()
				}
			}
			require.EqualValues(t, 1, counts.closes.Load())
		})
	}
}
