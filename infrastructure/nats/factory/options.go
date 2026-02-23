// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

//go:generate go run github.com/altessa-s/go-atlas/tools/codegen/optgen generate

import (
	"log/slog"

	"github.com/altessa-s/go-atlas/observability/health"

	tlsfactory "github.com/altessa-s/go-atlas/security/tlsutils/factory"
)

// options contains Factory configuration.
type options struct {
	// logger sets the logger for the factory.
	logger            *slog.Logger
	healthCoordinator *health.Coordinator
	tlsFactory        *tlsfactory.Factory
}
