// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package nats provides NATS JetStream KeyValue storage for idempotency keys.
// Offers distributed storage with automatic TTL cleanup for cloud-native deployments.
//
// Example:
//
//	js, _ := jetstream.New(nc)
//	storage, _ := nats.New(js,
//	    nats.WithBucket("idempotency"),
//	    nats.WithMaxAge(24*time.Hour),
//	)
//	handler := idempotency.New(storage)
package nats
