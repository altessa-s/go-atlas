// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

//go:generate go tool optgen generate

// DefaultCollectionName is the collection used by New.
const DefaultCollectionName = "request_ids"

type options struct {
	collectionName string `optgen:"default=DefaultCollectionName" optval:"nonempty"`
}
