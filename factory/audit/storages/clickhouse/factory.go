// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"cmp"
	"context"
	"log/slog"

	"github.com/altessa-s/go-atlas/audit"

	coreslices "github.com/altessa-s/go-atlas-core/collections/slices"
	corecontext "github.com/altessa-s/go-atlas-core/context"
	coreerrs "github.com/altessa-s/go-atlas-core/errors"
	auditclickhouse "github.com/altessa-s/go-atlas/audit/storages/clickhouse"
	auditconfig "github.com/altessa-s/go-atlas/config/audit"
	auditfactory "github.com/altessa-s/go-atlas/factory/audit"
	corefactory "github.com/altessa-s/go-atlas/factory/core"
)

// ErrConnRequired is returned when the ClickHouse audit storage is built
// without a connection.
var ErrConnRequired = corefactory.Errorf(corefactory.ErrDependencyRequired, "audit: clickhouse connection is required for storage type clickhouse")

// MinRecommendedBatchSize is the smallest dispatch batch size the builder
// accepts without a warning: every flush creates a part the server has to
// merge.
const MinRecommendedBatchSize = 1000

// Builder returns the storage builder of the clickhouse type, ready for
// auditfactory.AuditorBuilder.UseStorage. The storage writes through conn,
// typically opened with factory/infrastructure/clickhouse; the caller owns it.
//
// Missing columns and indexes are added when the configuration opts in, then
// the table is checked against the expected schema, so drift surfaces at
// startup.
func Builder(conn auditclickhouse.Conn) auditfactory.StorageBuilder {
	return func(ctx context.Context, cfg *auditconfig.Config, logger *slog.Logger) (audit.Storage, error) {
		if conn == nil {
			return nil, ErrConnRequired
		}
		ch := cfg.Storage.ClickHouse
		if ch == nil {
			return nil, corefactory.Errorf(corefactory.ErrConfigRequired, "clickhouse storage configuration is required")
		}

		if size := cfg.Dispatch.BatchSize; size > 0 && size < MinRecommendedBatchSize {
			logger.Warn("audit dispatch batch size is small for ClickHouse: every flush creates a part to merge",
				"batchSize", size, "recommended", MinRecommendedBatchSize)
		}

		opts := []auditclickhouse.Option{
			auditclickhouse.WithTableName(ch.TableName),
			auditclickhouse.WithEngine(ch.Engine),
			auditclickhouse.WithCluster(ch.Cluster),
			auditclickhouse.WithMaxBatchSize(ch.MaxBatchSize),
			auditclickhouse.WithLogger(logger),
		}
		opts = coreslices.AppendIf(opts, ch.TimeRangeMode != "",
			auditclickhouse.WithTimeRangeMode(auditclickhouse.TimeRangeMode(ch.TimeRangeMode)))
		opts = coreslices.AppendIf(opts, ch.SchemaCheckMode != "",
			auditclickhouse.WithSchemaCheck(auditclickhouse.SchemaCheckMode(ch.SchemaCheckMode)))
		opts = coreslices.AppendIf(opts, ch.TTL > 0, auditclickhouse.WithTTL(ch.TTL))
		opts = coreslices.AppendIf(opts, ch.DDLTimeout > 0, auditclickhouse.WithDDLTimeout(ch.DDLTimeout))
		opts = coreslices.AppendIf(opts, ch.AutoCreateTable, auditclickhouse.WithAutoCreateTable())
		opts = coreslices.AppendIf(opts, ch.Final, auditclickhouse.WithFinal())

		storage, err := auditclickhouse.New(ctx, conn, opts...)
		if err != nil {
			return nil, coreerrs.WrapOperation(err, "create audit clickhouse storage")
		}

		// The check is bounded by DDLTimeout, so a hung server fails startup
		// instead of blocking it.
		checkCtx, cancel := corecontext.WithMaxTimeout(ctx, cmp.Or(ch.DDLTimeout, auditclickhouse.DefaultDDLTimeout))
		defer cancel()
		if err := storage.CheckSchema(checkCtx); err != nil {
			return nil, err
		}

		return storage, nil
	}
}
