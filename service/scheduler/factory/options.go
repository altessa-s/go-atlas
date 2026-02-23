// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

//go:generate go run github.com/altessa-s/go-atlas/tools/codegen/optgen generate

import (
	"log/slog"

	"github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/altessa-s/go-atlas/data/leadelect"
)

// options contains Factory configuration.
type options struct {
	// logger is the logger for operational visibility.
	logger *slog.Logger
	// leaderElector is the leader elector for distributed scheduling.
	leaderElector leadelect.LeaderElector `optgen:"notnil" optval:"nil"`
	// mongoDb is the MongoDB database for MongoDB storage backends.
	mongoDb *mongo.Database
	// redisClient is the Redis client for Redis storage backends.
	redisClient redis.UniversalClient `optgen:"notnil" optval:"nil"`
}
