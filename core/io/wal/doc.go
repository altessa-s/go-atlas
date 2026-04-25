// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package wal implements a segmented, append-only write-ahead log for
// crash-safe durability of opaque byte payloads.
//
// A WAL stores records in a sequence of fixed-capacity segment files. Each
// record is length-prefixed and protected by a CRC32 checksum, so torn
// writes from an unclean shutdown are detected and discarded during
// recovery. A background goroutine fsyncs the active segment on a
// configurable interval, trading throughput for a bounded loss window.
//
// The package is payload-agnostic: it does not know or care what the bytes
// mean. Higher-level components (dispatchers, outboxes, durable queues)
// layer their own encoding and semantics on top.
//
// Lifecycle:
//
//	w, recovered, err := wal.Open("/var/lib/app/wal",
//	    wal.WithFsyncInterval(2*time.Millisecond),
//	)
//	if err != nil { /* ... */ }
//	defer w.Close()
//
//	// Replay any records left over from a previous run.
//	for _, r := range recovered {
//	    if err := process(r.Payload); err == nil {
//	        w.Ack(r.Offset)
//	    }
//	}
//
//	// Normal operation.
//	off, err := w.Append(payload)
//	if err != nil { /* ... */ }
//	// ... eventually, after the payload is durably handled downstream:
//	w.Ack(off)
//
// Delivery is at-least-once: a record remains on disk until it is acked,
// and any unacked records from sealed segments (or from the active segment
// at the time of a crash) are returned by the next Open call. Consumers
// must therefore be idempotent with respect to replayed payloads.
//
// # Concurrency
//
// Append, Ack, Sync, Stats and Close are safe to call concurrently from
// multiple goroutines. Append serializes on an internal mutex to keep
// on-disk bytes and accounting consistent.
//
// # Crash safety
//
// Each record is written as [uint32 length][uint32 crc32][payload]. On
// recovery, readSegment scans each segment forward and stops at the first
// record whose header is incomplete, whose length is implausible, or whose
// payload fails CRC verification. Bytes after the last verified record are
// truncated from the segment file, so a torn write from an unclean shutdown
// is invisible to callers on the next Open. Delivery remains at-least-once:
// every record that reached the most recent fsync is replayed.
//
// # Performance
//
// Append holds an internal mutex for the duration of the file.Write
// syscall, which serializes concurrent producers at the disk's write
// latency. Header and payload are coalesced into a reusable scratch buffer
// so each Append issues exactly one write syscall. The background fsync
// goroutine holds the mutex only long enough to snapshot the active file
// handle, not for the Sync syscall itself. Ack is O(N) over sealed
// segments; the expected N is 1-2 in typical dispatcher configurations.
//
// For high-throughput producers that cannot tolerate the Append mutex as
// a throughput bottleneck, front the WAL with the batching engine in
// [github.com/altessa-s/go-atlas/service/dispatch],
// which accumulates items in memory and hands batches to this package.
package wal
