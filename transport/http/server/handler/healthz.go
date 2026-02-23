// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package handler

import (
	"github.com/altessa-s/go-atlas/transport/http/server/writer"
)

// HealthResponse is the response for health check endpoints.
type HealthResponse struct {
	Status string `json:"status"`
}

// K8sHealtz is a handler for Kubernetes liveness probes.
// It responds with a JSON [HealthResponse] containing status "ok".
func K8sHealtz(rw writer.ReadWriter) {
	_ = rw.Write(HealthResponse{Status: "ok"}) //nolint:errcheck // Best effort
}

// K8sReadyz is a handler for Kubernetes readiness probes.
// It responds with a JSON [HealthResponse] containing status "ok".
func K8sReadyz(rw writer.ReadWriter) {
	_ = rw.Write(HealthResponse{Status: "ok"}) //nolint:errcheck // Best effort
}
