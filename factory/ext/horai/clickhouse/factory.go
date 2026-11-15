// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"cmp"
	"context"
	"errors"
	"time"

	corecontext "github.com/altessa-s/go-atlas-core/context"
	coreerrs "github.com/altessa-s/go-atlas-core/errors"
	schedulerconfig "github.com/altessa-s/go-atlas/config/scheduler"
	corefactory "github.com/altessa-s/go-atlas/factory/core"
	schedulerfactory "github.com/altessa-s/go-atlas/factory/ext/horai"
	horai "github.com/altessa-s/go-horai"
	chstorage "github.com/altessa-s/go-horai/storage/clickhouse"
)

// ErrConnRequired is returned when the ClickHouse history storage is built
// without a connection.
var ErrConnRequired = errors.New("scheduler: clickhouse connection is required for history storage type clickhouse")

// ensureTimeout bounds the schema creation the builder runs when the config
// asks for it, so a lock wait cannot stall startup indefinitely.
const ensureTimeout = 30 * time.Second

// Builder returns the history storage builder of the clickhouse type, ready
// for schedulerfactory.SchedulerBuilder.UseHistoryStorage. The storage writes
// through conn, typically opened with factory/infrastructure/clickhouse; the
// caller owns it.
//
// A zero TTL takes the scheduler's history retention. With ensureSchema set it
// creates the table through the storage's EnsureSchema; otherwise it performs
// no I/O and the table is expected to exist.
func Builder(conn chstorage.Conn) schedulerfactory.HistoryStorageBuilder {
	return func(ctx context.Context, cfg *schedulerconfig.Config) (horai.HistoryStore, error) {
		if conn == nil {
			return nil, ErrConnRequired
		}
		ch := cfg.HistoryStorage.ClickHouse
		if ch == nil {
			return nil, corefactory.Errorf(corefactory.ErrConfigRequired, "clickhouse history storage configuration is required")
		}

		storage, err := chstorage.New(conn,
			chstorage.WithTableName(ch.TableName),
			chstorage.WithEngine(ch.Engine),
			chstorage.WithCluster(ch.Cluster),
			chstorage.WithTTL(cmp.Or(ch.TTL, cfg.HistoryRetention)),
		)
		if err != nil {
			return nil, coreerrs.WrapOperation(err, "create scheduler history storage")
		}
		if !ch.EnsureSchema {
			return storage, nil
		}

		ensureCtx, cancel := corecontext.WithMaxTimeout(ctx, ensureTimeout)
		defer cancel()
		if err := storage.EnsureSchema(ensureCtx); err != nil {
			return nil, coreerrs.WrapOperation(err, "ensure scheduler history schema")
		}
		return storage, nil
	}
}
