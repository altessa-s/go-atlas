// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package auth

//go:generate go run github.com/altessa-s/go-atlas/tools/codegen/optgen generate

import (
	"log/slog"
	"time"
)

// Default configuration for authentication retries.
const (
	// DefaultBackoffBase is the base duration for retry backoff.
	DefaultBackoffBase = 2 * time.Second
	// DefaultBackoffMax is the maximum duration for retry backoff.
	DefaultBackoffMax = 30 * time.Second
	// DefaultAuthTimeout is the timeout for individual authentication operations.
	DefaultAuthTimeout = 30 * time.Second
)

type options struct {
	logger      *slog.Logger
	backoffBase time.Duration `optgen:"default=DefaultBackoffBase"`
	backoffMax  time.Duration `optgen:"default=DefaultBackoffMax"`
	authTimeout time.Duration `optgen:"default=DefaultAuthTimeout"`
}
