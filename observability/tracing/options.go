// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

//go:generate go run github.com/altessa-s/go-atlas/cmd/optgen generate

package tracing

import (
	"github.com/altessa-s/go-atlas/observability/tracing/adapters"
	"github.com/altessa-s/go-atlas/observability/tracing/sampler"

	_ "github.com/altessa-s/go-atlas/core/runtime/appinfo"
)

// options contains configuration for Tracer.
type options struct {
	// serviceName is the service name attached to all spans.
	serviceName string `optgen:"default=appinfo.Name"`

	// serviceVersion is the service version attached to all spans.
	serviceVersion string `optgen:"default=appinfo.Version"`

	// environment is the deployment environment (e.g., "production", "staging").
	environment string `optgen:"default=appinfo.EnvLabel()"`

	// adapter receives span exports.
	// For multiple backends, use adapters.NewMultiAdapter().
	adapter adapters.Adapter

	// sampler determines which spans are recorded.
	sampler sampler.Sampler
}
