// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package keyset reads MongoDB pages using bound continuation tokens.
//
// FindPage[T] reads a MongoDB page through the Collection port and optionally issues a continuation token through TokenCodec. Query
// carries filter, sort, projection, size, bindings, registry, and BSON options. Import this package directly; pagination types are not
// re-exported from store/mongo.
//
// # Usage
//
//	codec, err := pagetoken.New(key /*, options */)
//	page, err := keyset.FindPage[User](ctx, collection, codec, keyset.Query{Limit: 100})
//
// See README.md for the complete guide, limits, errors, and extension contracts.
package keyset
