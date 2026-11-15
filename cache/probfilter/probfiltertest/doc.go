// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package probfiltertest provides a shared in-memory Bloom storage for tests of
// probfilter factory consumers that exercise the shared-filter rebuild lease
// without Redis.
//
// Example, registered as the redis type:
//
//	f, err := probfilterfactory.NewFilter("users", cfg, defaults).
//		UseBloomStorage(probfilterconfig.StorageTypeRedis,
//			func(context.Context, probfilterfactory.BloomStorageSpec) (bloomstorages.Storage, error) {
//				return probfiltertest.NewSharedBloom(false), nil
//			}).
//		Build(ctx)
package probfiltertest
