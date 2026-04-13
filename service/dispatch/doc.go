// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package dispatch provides a generic, non-blocking, batching dispatch
// engine with optional crash-safe persistence backed by
// [github.com/altessa-s/go-atlas/core/io/wal].
//
// The engine is designed for high-throughput "fire-and-forget" producers
// (audit logs, request access logs, telemetry) where the hot path must not
// block on I/O but events must still survive process crashes.
//
// # Hot path
//
// Without WAL, Engine.Submit only sends the item to an in-memory channel and
// returns. With WAL enabled, Submit additionally serializes the item via the
// configured Codec, appends it to the active WAL segment (page-cache write,
// no fsync), and then sends it to the channel. A background goroutine fsyncs
// the active segment on the interval configured via wal.WithFsyncInterval
// (group commit), bounding the loss window after a crash.
//
// # Workers
//
// Worker goroutines drain the channel, build batches up to BatchSize or
// FlushInterval, and call Sink.StoreBatch. On success, the corresponding WAL
// records are acked; once all records in a sealed segment are acked, the
// segment file is removed.
//
// # Recovery
//
// On Engine.Start, the WAL directory is scanned, every valid record is
// decoded via Codec, and the records are pushed back into the worker queue
// before any new Submit calls are accepted by the workers. Torn writes
// (partial records left after a crash) are detected by CRC32 and discarded.
//
// # Durability guarantees
//
//   - Graceful Shutdown: drains the queue and waits for the sink, then closes
//     the WAL. Acked segments are removed; nothing is lost.
//   - Crash (SIGKILL/panic/OOM): records that reached the most recent fsync
//     survive and are replayed on next start. The loss window is bounded by
//     FsyncInterval.
//
// # Disabling the WAL
//
// Pass no WithWAL option to keep the engine purely in-memory. In that mode
// the engine is equivalent to a generic batching dispatcher with no I/O on
// the submit path; the Codec field is not required.
//
// # Performance
//
// Submit is allocation-free on the no-WAL hot path: the envelope is a
// small value type sent through a buffered channel, and back-pressure is
// configurable via WithBackPressure. With WAL enabled, each Submit pays
// one codec encode plus a mutex-serialized file.Write inside the WAL
// package — see the Performance section of
// [github.com/altessa-s/go-atlas/core/io/wal] for details on the
// Append mutex trade-off.
//
// Worker goroutines reuse their batch and offset slices via zero-length
// reslicing (batch[:0]) across flushes, so the steady-state worker cost
// is one ticker interrupt per FlushInterval plus the per-batch sink call.
// Encode errors and full-buffer drops both fall through e.dropItem,
// incrementing the dropped metric and firing the OnDrop callback, so
// observability is uniform across drop causes.
//
// For the dropped-item semantics, sinkErrors is counted at item
// granularity (not batch granularity), matching the enqueued counter so
// consumers can reason about loss rate from a single pair of metrics.
package dispatch
