// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

//go:generate go run github.com/altessa-s/go-atlas/tools/codegen/optgen generate

import (
	"log/slog"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/redis/go-redis/v9"

	corescheduler "github.com/altessa-s/go-atlas/core/scheduler"
)

// options contains Factory configuration.
type options struct {
	// logger is the logger for operational visibility.
	logger *slog.Logger
	// redisClient is the Redis client for Redis storage backends.
	redisClient redis.UniversalClient `optgen:"notnil"`
	// jetstream is the NATS JetStream context for NATS storage backends.
	jetstream jetstream.JetStream `optgen:"notnil"`
	// scheduler is the scheduler for background task registration.
	scheduler corescheduler.TaskRegistrar `optgen:"notnil"`
}
