// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

//go:generate go run github.com/altessa-s/go-atlas/tools/codegen/optgen generate

import (
	"log/slog"

	"github.com/altessa-s/go-atlas/observability/tracing"

	idempotencydata "github.com/altessa-s/go-atlas/data/idempotency"
	sharedlimiter "github.com/altessa-s/go-atlas/data/limiters"
	tlsproviders "github.com/altessa-s/go-atlas/security/tlsutils/providers"
)

// options contains Factory configuration.
type options struct {
	// logger is the logger for operational visibility.
	logger       *slog.Logger
	tlsProviders *tlsproviders.Providers
	tracer       tracing.Tracer              `optgen:"notnil" optval:"nil"`
	limiter      sharedlimiter.Limiter       `optgen:"notnil" optval:"nil"`
	idempotency  idempotencydata.Idempotency `optgen:"notnil" optval:"nil"`
}
