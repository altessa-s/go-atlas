// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package livebackend holds the fixtures that more than one integration suite
// needs to reach a live backend: the Redis address with its environment
// override, a client that skips the test when the server is unreachable, and
// per-test names and key prefixes that are cleaned up afterwards.
//
// Several suites, and several test processes, share one Redis server. Every
// test therefore works under a key prefix of its own, derived from its name
// and a random suffix, and deletes exactly those keys on cleanup — never the
// whole database.
//
//	client := livebackend.ConnectRedis(t)
//	prefix := livebackend.RedisPrefix(t, client, "cacheit")
//	storage := redisstorage.New(client, redisstorage.WithPrefix(prefix))
package livebackend
