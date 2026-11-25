// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/transport/http/server/router"
	"github.com/altessa-s/go-atlas/transport/http/server/router/std"

	httpconfig "github.com/altessa-s/go-atlas/config/http"
	factory "github.com/altessa-s/go-atlas/factory/transport/http/server"
)

func TestCustomRouterEndpoints(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name           string
		pprof, metrics bool
	}{
		{"both", true, true}, {"metrics only", false, true}, {"pprof only", true, false}, {"disabled", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := httpconfig.Default()
			custom := struct{ router.Router }{std.New()}
			metrics := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusAccepted)
			})
			b := factory.New(&cfg).WithRouter(custom).WithInternalPrefix("/ops").UseMetricsHandler(metrics)
			if tc.pprof {
				b.WithPprof()
			}
			if !tc.metrics {
				b.WithoutMetrics()
			}
			base := start(t, b)
			for _, endpoint := range []struct {
				path    string
				enabled bool
				status  int
			}{
				{"/ops/pprof/", tc.pprof, http.StatusOK},
				{"/ops/metrics/", tc.metrics, http.StatusAccepted},
				{"/internal/metrics/", false, 0},
				{"/internal/pprof/", false, 0},
			} {
				resp, _ := get(t, base+endpoint.path, nil)
				want := http.StatusNotFound
				if endpoint.enabled {
					want = endpoint.status
				}
				require.Equal(t, want, resp.StatusCode, endpoint.path)
			}
		})
	}
}
