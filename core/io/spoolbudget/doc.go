// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package spoolbudget bounds the aggregate disk held by concurrent spools.
//
// A [github.com/altessa-s/go-atlas/core/io/spool.Spool] caps each materialized
// reader individually, but without coordination the concurrent sum of on-disk
// bytes is bounded only by the worker parallelism times the per-spool cap. A
// single process-wide [Budget] charges every spool's materialized size against a
// shared byte ceiling and blocks (back-pressure) until the reservation fits,
// releasing it when the spool is closed. This makes the budget a true peak
// bound: a spool reserves its whole cap before writing, so the sum of on-disk
// bytes across all in-flight spools never exceeds the limit.
package spoolbudget
