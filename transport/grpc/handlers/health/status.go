// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package health

import (
	"github.com/altessa-s/go-atlas/observability/health"

	"google.golang.org/grpc/health/grpc_health_v1"
)

// ToProto maps an [health.ServingStatus] to the corresponding
// grpc_health_v1 enum value. Unrecognized values map to UNKNOWN.
func ToProto(s health.ServingStatus) grpc_health_v1.HealthCheckResponse_ServingStatus {
	switch s {
	case health.StatusServing:
		return grpc_health_v1.HealthCheckResponse_SERVING
	case health.StatusNotServing:
		return grpc_health_v1.HealthCheckResponse_NOT_SERVING
	case health.StatusServiceUnknown:
		return grpc_health_v1.HealthCheckResponse_SERVICE_UNKNOWN
	default:
		return grpc_health_v1.HealthCheckResponse_UNKNOWN
	}
}
