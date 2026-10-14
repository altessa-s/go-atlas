// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package probfilterit runs the Redis storages of data/probfilter — the Bloom
// (BF.*) and Cuckoo (CF.*) filters behind their facades — against a live
// Redis Stack server with the RedisBloom module.
//
// The unit tests run on miniredis, which has no RedisBloom commands, so they
// stop at structure: options, key layout, and Lua scripts on plain keys. The
// filter commands themselves, the error texts the storages recognize ("not
// found" from a NOCREATE insert or an INFO of a missing filter), and every
// script that wraps a filter command — the reserve script, the journaled
// insert, the staging reserve, the journal drain, the commit with its journal
// replay, the rebuild lease and the generation-fenced delete — only run here.
//
// The scenarios cover membership, batched inserts larger than one command,
// a false-positive rate measured over a few thousand items, deletes, and
// rebuilds: staged and committed atomically, failed without touching the live
// filter, refused while another instance holds the rebuild lease, discarded
// once superseded, and keeping the inserts other instances make meanwhile.
// Two facade instances on their own connections stand in for two processes
// sharing one filter.
//
// Tests skip rather than fail when Redis is unreachable or lacks RedisBloom.
// Every test works under its own key prefix; the filter keys and their
// "__probfilter__:" metadata keys are deleted on cleanup. See
// tests/integration/README.md.
package probfilterit
