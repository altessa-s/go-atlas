// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

//go:generate go tool optgen generate

package tracing

import (
	"time"

	"github.com/altessa-s/go-atlas/observability/metrics"
	"github.com/altessa-s/go-atlas/observability/tracing/adapters"
	"github.com/altessa-s/go-atlas/observability/tracing/sampler"

	_ "github.com/altessa-s/go-atlas-core/runtime/appinfo"
)

// Default batch export settings.
const (
	DefaultMaxQueueSize       = 2048
	DefaultMaxExportBatchSize = 512
	DefaultScheduledDelay     = time.Second
	DefaultExportTimeout      = 10 * time.Second
)

// Default span limit settings matching the OpenTelemetry specification (0 means unlimited).
const (
	DefaultMaxAttributes = 128
	DefaultMaxEvents     = 128
	DefaultMaxLinks      = 128
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

	// maxQueueSize limits sampled spans waiting for export. A full queue drops new spans.
	maxQueueSize int `optgen:"default=DefaultMaxQueueSize" optval:"positive"`

	// maxExportBatchSize limits spans per adapter call, capped at maxQueueSize.
	maxExportBatchSize int `optgen:"default=DefaultMaxExportBatchSize" optval:"positive"`

	// scheduledDelay is the interval for exporting partial batches.
	scheduledDelay time.Duration `optgen:"default=DefaultScheduledDelay" optval:"positive"`

	// exportTimeout is the deadline for each batch export.
	exportTimeout time.Duration `optgen:"default=DefaultExportTimeout" optval:"positive"`

	// meter records exporter metrics under the tracing subsystem.
	meter metrics.Meter `optgen:"notnil"`

	// maxAttributes limits attributes retained per span (0 means unlimited).
	maxAttributes int `optgen:"default=DefaultMaxAttributes" optval:"positive=allow_zero"`

	// maxEvents limits events retained per span (0 means unlimited).
	maxEvents int `optgen:"default=DefaultMaxEvents" optval:"positive=allow_zero"`

	// maxLinks limits links retained per span (0 means unlimited).
	maxLinks int `optgen:"default=DefaultMaxLinks" optval:"positive=allow_zero"`
}
