// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"context"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/altessa-s/go-aip/requestid"

	"github.com/altessa-s/go-atlas-core/types/nilcheck"

	config "github.com/altessa-s/go-atlas/config/requestidstore"
	corefactory "github.com/altessa-s/go-atlas/factory/core"
	storefactory "github.com/altessa-s/go-atlas/factory/reliability/requestidstore"
	backend "github.com/altessa-s/go-atlas/reliability/requestidstore/storages/nats"
)

// Builder binds the nats client for use with StoreBuilder.UseStorage.
func Builder(js jetstream.JetStream) storefactory.StorageFunc {
	return func(ctx context.Context, cfg *config.Config) (requestid.Store, error) {
		if nilcheck.IsNil(js) {
			return nil, corefactory.Errorf(corefactory.ErrDependencyRequired, "nats client required")
		}
		if cfg == nil || cfg.NATS == nil {
			return nil, corefactory.ErrConfigRequired
		}
		c := cfg.NATS
		return backend.New(ctx, js, backend.WithBucket(c.Bucket), backend.WithReplicas(c.Replicas), backend.WithPurgeInterval(c.PurgeInterval))
	}
}
