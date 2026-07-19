// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package redisfilter provides the shared plumbing for Redis-backed
// probabilistic filter storages built on RedisBloom module commands.
//
// The Bloom (BF.*) and Cuckoo (CF.*) storages differ almost only in the
// command prefix, the reserve arguments, and the filter label used in
// wrapped errors. [Core] captures that common shape: command execution,
// error wrapping, batching, and create-on-first-use (ensure-filter)
// behavior, parameterized by a [Commands] set.
//
// # Core
//
// A storage embeds or holds a [Core] configured with its command set:
//
//	core := redisfilter.New(client, "bloom:myfilter", redisfilter.Commands{
//	    Label:    "Bloom",
//	    Exists:   "BF.EXISTS",
//	    Add:      "BF.ADD",
//	    AddBatch: "BF.MADD",
//	    Reserve:  "BF.RESERVE",
//	    Info:     "BF.INFO",
//	}, falsePositiveRate, expectedItems)
//
//	ok, err := core.MightExist(ctx, "value")
//	err = core.Add(ctx, "value")
//
// # Info parsing
//
// [Core.Info] returns the raw *.INFO reply; [InfoFields] iterates its
// alternating key/value pairs and [ToInt64] converts values, leaving the
// filter-specific field mapping to the caller:
//
//	for key, raw := range redisfilter.InfoFields(result) {
//	    if key == "Capacity" {
//	        capacity, _ = redisfilter.ToInt64(raw)
//	    }
//	}
package redisfilter
