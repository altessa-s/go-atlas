// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package health provides HTTP handlers for liveness and readiness probes.
//
// Two flavors are available:
//
//   - Static stubs ([K8sHealtz], [K8sReadyz]) always return 200 OK and have
//     no dependencies. Suitable for the simplest deployments where the only
//     signal that matters is "the process is up and accepting connections".
//   - Coordinator-backed handlers ([Healthz], [Readyz], [Detailed]) consult
//     a [observability/health.Coordinator] and translate its
//     [observability/health.ServingStatus] into HTTP 200/503 plus a JSON
//     body. Use these once your dependencies report health into a
//     coordinator (see docs/health.md).
//
// # Usage
//
//	import healthh "github.com/altessa-s/go-atlas/transport/http/server/handlers/health"
//
//	srv.Handle("/internal/healthz", healthh.K8sHealtz)
//	srv.Handle("/internal/readyz",  healthh.Readyz(coordinator))
package health
