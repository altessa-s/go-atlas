// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package bsonfield connects a document writer's per-call field transformation
// to the MongoDB translator.
//
// # Usage
//
//	ctx = bsonfield.WithTransformer(ctx, transformer)
package bsonfield
