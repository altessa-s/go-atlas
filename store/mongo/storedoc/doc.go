// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package storedoc classifies BSON decoding failures at a storage boundary
// without prescribing the application's error taxonomy.
//
// # Usage
//
// The caller supplies the sentinel used for corrupt stored records:
//
//	err := collection.FindOne(ctx, filter).Decode(&document)
//	return storedoc.Classify(err, ErrCorruptRecord)
//
// Query and transport errors pass through unchanged. Classified failures keep
// both the caller's category and the original decoder error in their chain.
package storedoc
