// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"fmt"
	"log/slog"

	"github.com/altessa-s/go-atlas/config"
	"github.com/altessa-s/go-atlas/core/io/wal"
	"github.com/altessa-s/go-atlas/service/dispatch"

	coreslices "github.com/altessa-s/go-atlas/core/collections/slices"
	corefactory "github.com/altessa-s/go-atlas/core/factory"
)

// EngineBuilder assembles a [dispatch.Engine] from configuration using a
// fluent API. Create instances with [New]. Errors are accumulated and
// reported at [EngineBuilder.Build] time. The builder is not safe for
// concurrent use.
type EngineBuilder[T any] struct {
	corefactory.Base
	cfg  *config.DispatchConfig
	errs []error

	// Dependencies
	sink  dispatch.Sink[T]
	codec dispatch.Codec[T]
}

// New creates an [EngineBuilder] for the given dispatch config.
// Config can be nil — the error surfaces at [EngineBuilder.Build] time.
func New[T any](cfg *config.DispatchConfig) *EngineBuilder[T] {
	return &EngineBuilder[T]{
		Base: corefactory.NewBase(slog.New(slog.DiscardHandler)),
		cfg:  cfg,
	}
}

// WithSink sets the [dispatch.Sink] that the engine delivers batches to.
func (b *EngineBuilder[T]) WithSink(s dispatch.Sink[T]) *EngineBuilder[T] {
	b.sink = s
	return b
}

// WithCodec sets the [dispatch.Codec] used for WAL serialization. Required
// when WAL is enabled; ignored otherwise.
func (b *EngineBuilder[T]) WithCodec(c dispatch.Codec[T]) *EngineBuilder[T] {
	b.codec = c
	return b
}

// WithLogger sets the logger for the builder and the engine.
func (b *EngineBuilder[T]) WithLogger(v *slog.Logger) *EngineBuilder[T] {
	b.SetLogger(v)
	return b
}

// Build constructs, configures, and starts the [dispatch.Engine].
// The engine is ready to accept [dispatch.Engine.Submit] calls on return.
func (b *EngineBuilder[T]) Build() (*dispatch.Engine[T], error) {
	if err := corefactory.JoinErrors(b.errs); err != nil {
		return nil, err
	}
	if b.cfg == nil {
		return nil, fmt.Errorf("dispatch: configuration is required")
	}
	if b.sink == nil {
		return nil, fmt.Errorf("dispatch: sink is required")
	}

	eng, err := b.buildEngine()
	if err != nil {
		return nil, err
	}
	if err := eng.Start(); err != nil {
		return nil, fmt.Errorf("dispatch: start engine: %w", err)
	}
	return eng, nil
}

func (b *EngineBuilder[T]) buildEngine() (*dispatch.Engine[T], error) {
	cfg := b.cfg

	opts := []dispatch.Option[T]{
		dispatch.WithBufferSize[T](cfg.BufferSize),
		dispatch.WithBatchSize[T](cfg.BatchSize),
		dispatch.WithWorkers[T](cfg.Workers),
		dispatch.WithRetryAttempts[T](cfg.RetryAttempts),
		dispatch.WithLogger[T](b.Logger()),
	}
	opts = coreslices.AppendIf(opts, cfg.FlushInterval > 0, dispatch.WithFlushInterval[T](cfg.FlushInterval))
	opts = coreslices.AppendIf(opts, cfg.RetryBackoff > 0, dispatch.WithRetryBackoff[T](cfg.RetryBackoff))
	opts = coreslices.AppendIf(opts, cfg.BackPressure, dispatch.WithBackPressure[T]())
	opts = coreslices.AppendIf(opts, cfg.MetricsSubsystem != "", dispatch.WithMetricsSubsystem[T](cfg.MetricsSubsystem))

	if cfg.WAL.Enabled {
		if b.codec == nil {
			return nil, fmt.Errorf("dispatch: codec is required when WAL is enabled")
		}
		var walOpts []wal.Option
		walOpts = coreslices.AppendIf(walOpts, cfg.WAL.MaxSegmentBytes > 0, wal.WithMaxSegmentBytes(cfg.WAL.MaxSegmentBytes))
		walOpts = coreslices.AppendIf(walOpts, cfg.WAL.MaxBytes > 0, wal.WithMaxBytes(cfg.WAL.MaxBytes))
		walOpts = coreslices.AppendIf(walOpts, cfg.WAL.FsyncInterval > 0, wal.WithFsyncInterval(cfg.WAL.FsyncInterval))
		opts = append(opts, dispatch.WithWAL[T](cfg.WAL.Dir, b.codec, walOpts...))
	}

	return dispatch.NewEngine[T](b.sink, opts...)
}
