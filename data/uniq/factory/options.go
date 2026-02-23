// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

//go:generate go run github.com/altessa-s/go-atlas/tools/codegen/optgen generate

import (
	"log/slog"

	"github.com/nats-io/nats.go"
	"github.com/redis/go-redis/v9"
)

// options contains Factory configuration.
type options struct {
	// logger is the logger for operational visibility.
	logger *slog.Logger
	// redisClient is the Redis client for Redis provider.
	redisClient redis.UniversalClient
	// natsConn is the NATS connection for NATS provider.
	natsConn *nats.Conn
}
