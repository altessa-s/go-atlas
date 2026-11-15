// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package bsoncodec supplies MongoDB codecs for core value types without
// making those value types depend on the database driver.
//
// # Usage
//
//	reg := bsoncodec.NewRegistry()
//	client, err := mongo.Connect(options.Client().SetRegistry(reg))
//
// Optional values encode as their inner value or BSON null. None fields tagged
// omitempty are omitted. Some(nil) encodes as null and decodes as None; BSON
// cannot distinguish these states. Inner values use the caller's registry.
//
// Decimals are stored as exact Decimal128 values or rejected with
// ErrDecimalOutOfRange. The decimal128.Fits function from
// github.com/altessa-s/go-aip/decimal128 reports the same verdict up front,
// so input validation rejects exactly what the encoder would:
//
//	if !decimal128.Fits(amount) {
//	    return coreerrs.NewFieldError("amount", "not representable as Decimal128")
//	}
package bsoncodec
