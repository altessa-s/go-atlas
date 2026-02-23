// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package defaults

import (
	"regexp"

	"github.com/altessa-s/go-atlas/transport/internal/endpointfilter"
)

// IgnorePatterns contains the default regex patterns to ignore for all interceptors.
// By default, gRPC reflection and health check service methods are excluded.
//
// This includes:
//   - /grpc.reflection.v1alpha.ServerReflection/* - gRPC reflection service
//   - /grpc.health.v1.Health/* - gRPC health check service
//
// Interceptors can use this as a default and extend it with additional patterns
// specific to their needs.
var IgnorePatterns = []*regexp.Regexp{
	endpointfilter.ReflectionMethodPattern,
	endpointfilter.HealthMethodPattern,
}
