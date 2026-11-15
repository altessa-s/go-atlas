// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"testing"

	"github.com/stretchr/testify/require"

	bodylimitmw "github.com/altessa-s/go-atlas/transport/http/server/middlewares/bodylimit"
	corsmw "github.com/altessa-s/go-atlas/transport/http/server/middlewares/cors"
	geoaclmw "github.com/altessa-s/go-atlas/transport/http/server/middlewares/geoacl"
	idempotencymw "github.com/altessa-s/go-atlas/transport/http/server/middlewares/idempotency"
	ipaclmw "github.com/altessa-s/go-atlas/transport/http/server/middlewares/ipacl"
	limitermw "github.com/altessa-s/go-atlas/transport/http/server/middlewares/limiter"
	loggermw "github.com/altessa-s/go-atlas/transport/http/server/middlewares/logger"
	metricsmw "github.com/altessa-s/go-atlas/transport/http/server/middlewares/metrics"
	realipmw "github.com/altessa-s/go-atlas/transport/http/server/middlewares/realip"
	recoverymw "github.com/altessa-s/go-atlas/transport/http/server/middlewares/recovery"
	requestidmw "github.com/altessa-s/go-atlas/transport/http/server/middlewares/requestid"
	securityheadersmw "github.com/altessa-s/go-atlas/transport/http/server/middlewares/securityheaders"
	tracingmw "github.com/altessa-s/go-atlas/transport/http/server/middlewares/tracing"
)

// TestWithoutMiddleware_DisablesByRealName guards against a Without*Middleware
// method disabling a name no middleware carries (WithoutPrometheusMiddleware
// once disabled "prometheus" while the metrics middleware is named "metrics").
func TestWithoutMiddleware_DisablesByRealName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		disable func(*ServerBuilder) *ServerBuilder
		want    string
	}{
		{"bodylimit", (*ServerBuilder).WithoutBodyLimitMiddleware, bodylimitmw.Name()},
		{"cors", (*ServerBuilder).WithoutCorsMiddleware, corsmw.Name()},
		{"idempotency", (*ServerBuilder).WithoutIdempotencyMiddleware, idempotencymw.Name()},
		{"ipacl", (*ServerBuilder).WithoutIpAclMiddleware, ipaclmw.Name()},
		{"geoacl", (*ServerBuilder).WithoutGeoAclMiddleware, geoaclmw.Name()},
		{"limiter", (*ServerBuilder).WithoutLimiterMiddleware, limitermw.Name()},
		{"logger", (*ServerBuilder).WithoutLoggerMiddleware, loggermw.Name()},
		{"metrics", (*ServerBuilder).WithoutPrometheusMiddleware, metricsmw.Name()},
		{"tracing", (*ServerBuilder).WithoutTracingMiddleware, tracingmw.Name()},
		{"realip", (*ServerBuilder).WithoutRealIPMiddleware, realipmw.Name()},
		{"recovery", (*ServerBuilder).WithoutRecoveryMiddleware, recoverymw.Name()},
		{"requestid", (*ServerBuilder).WithoutRequestIDMiddleware, requestidmw.Name()},
		{"securityheaders", (*ServerBuilder).WithoutSecurityHeadersMiddleware, securityheadersmw.Name()},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			b := tc.disable(New(nil))

			require.Contains(t, b.disabledMW, tc.want)
			require.Len(t, b.disabledMW, 1)
		})
	}
}
