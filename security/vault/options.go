// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package vault

//go:generate go run github.com/altessa-s/go-atlas/tools/codegen/optgen generate

import (
	"crypto/tls"
	"log/slog"
	"time"

	"github.com/altessa-s/go-atlas/observability/health"
	"github.com/altessa-s/go-atlas/security/vault/auth"

	vaultApi "github.com/hashicorp/vault/api"
)

// DefaultInitialAuthTimeout is the default timeout for initial authentication.
const DefaultInitialAuthTimeout = 10 * time.Second

type options struct {
	vaultClient       *vaultApi.Client
	tlsConfig         *tls.Config
	authMethod        auth.Method `opgen:"nonnil"`
	logger            *slog.Logger
	authTimeout       time.Duration `optgen:"default=DefaultInitialAuthTimeout"`
	healthCoordinator *health.Coordinator
}
