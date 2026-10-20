// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package nats provides NATS JetStream KeyValue rate limit storage for distributed tokenbucket deployments.
// Uses sliding window algorithm with automatic bucket-level TTL cleanup.
//
// KeyValue cannot update several keys atomically, so the provider does not
// implement storages.MultiStorage, and storages.AllowAll returns
// storages.ErrMultiKeyUnsupported for it.
//
// Example:
//
//	nc, _ := nats.Connect(nats.DefaultURL)
//	js, _ := jetstream.New(nc)
//	storage, _ := nats.New(js, nats.WithBucket("rate-limiter"))
//	defer storage.Close()
package nats
