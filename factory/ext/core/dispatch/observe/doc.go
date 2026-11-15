// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package observe implements a [dispatch.Observer] that records the events of
// a go-atlas-core dispatch engine as metrics. It keeps the engine free of the
// observability stack: the engine only emits events, this package turns them
// into [metrics.Meter] instruments.
//
// Create one [Observer] per engine with [New] and install it with
// [dispatch.WithObserver].
//
// # Metrics
//
// All metrics are registered under the [WithMetricsSubsystem] subsystem
// (default [DefaultMetricsSubsystem], "async"):
//
//   - items_enqueued_total: items admitted to the buffer or the WAL.
//   - items_dropped_total: items rejected on admission.
//   - batch_flush_duration_seconds: time a batch spent in the sink, retries
//     included.
//   - sink_errors_total: items of failed sink attempts, retries included.
//   - wal_errors_total: WAL append failures.
//   - encode_errors_total: codec encode failures.
//   - workers_active: running dispatch workers.
//   - wal_bytes: bytes held by the WAL across all segments.
//   - wal_replay_total: records replayed from the WAL on startup.
//
// # Usage
//
//	eng, err := dispatch.NewEngine[Event](sink,
//		dispatch.WithObserver[Event](observe.New(meter)),
//	)
package observe
