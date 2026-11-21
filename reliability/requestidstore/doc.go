// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package requestidstore provides storage backends for AIP-155 request IDs handled by the go-aip Guard.
//
// # Usage
//
//	store := redis.New(client)
//	guard, err := requestid.New(store)
package requestidstore
