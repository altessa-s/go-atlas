// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package health

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/observability/health"

	"google.golang.org/grpc/health/grpc_health_v1"
)

func TestToProto(t *testing.T) {
	tests := []struct {
		name   string
		status health.ServingStatus
		want   grpc_health_v1.HealthCheckResponse_ServingStatus
	}{
		{"serving", health.StatusServing, grpc_health_v1.HealthCheckResponse_SERVING},
		{"not_serving", health.StatusNotServing, grpc_health_v1.HealthCheckResponse_NOT_SERVING},
		{"service_unknown", health.StatusServiceUnknown, grpc_health_v1.HealthCheckResponse_SERVICE_UNKNOWN},
		{"unknown", health.StatusUnknown, grpc_health_v1.HealthCheckResponse_UNKNOWN},
		{"default_for_invalid", health.ServingStatus(99), grpc_health_v1.HealthCheckResponse_UNKNOWN},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ToProto(tt.status)
			require.Equal(t, tt.want, got)
		})
	}
}
