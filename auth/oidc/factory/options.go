// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

//go:generate go run github.com/altessa-s/go-atlas/tools/codegen/optgen generate

import (
	"log/slog"

	"github.com/redis/go-redis/v9"

	"github.com/altessa-s/go-atlas/auth/oidc"

	corescheduler "github.com/altessa-s/go-atlas/core/scheduler"
)

// options contains Factory configuration.
type options struct {
	logger      *slog.Logger
	scheduler   corescheduler.TaskRegistrar `optgen:"notnil"`
	redisClient redis.UniversalClient       `optgen:"notnil"`
	tokenCache  oidc.Cacher                 `optgen:"notnil"`
}
