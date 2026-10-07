// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package storageconfig defines shared storage-backend schemas: cache storage
// selection (memory, NATS, Redis) and NATS JetStream KeyValue settings.
//
// Schemas carry yaml and default tags read by
// github.com/altessa-s/go-atlas/config/loader, expose Default* constructors
// where defaults exist, and implement Validate; component factories map them
// to generated options.
//
// Key types: [KVStorageType], [NATSConfig], [RedisConfig],
// [MemoryConfig], [CacheStorageType], [CacheStorageConfig].
package storageconfig
