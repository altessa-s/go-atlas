// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package optionalcodec provides a converter codec that bridges
// optional.Optional[T] fields and matching pointer or value
// counterparts during struct-to-struct conversion.
//
// Register it with the converter (and, by extension, the data/mongo
// helpers) via WithCodecs:
//
//	conv := converter.NewShared[ModelT, EntityT](
//	    converter.WithCodecs(optionalcodec.Codec),
//	)
//
//	// or, through data/mongo:
//	m, err := mongo.New("mydb",
//	    mongo.WithConverterOptions(
//	        converter.WithCodecs(optionalcodec.Codec),
//	    ),
//	)
//
// # Supported shapes
//
//   - Optional[T] ↔ Optional[T]: direct copy.
//   - Optional[T] ↔ *T:           None ↔ nil; Some(v) ↔ &v.
//   - Optional[T] ↔ T:            None ↔ zero T; Some(v) ↔ v.
//
// Unhandled field pairs are delegated back to the codec chain.
//
// The codec uses the reflection escape hatch exposed by
// core/types/optional (IsOptionalType, InnerType, GetReflect,
// SomeReflect, NoneReflect) and never accesses Optional's internal
// layout directly. It is safe for concurrent use.
package optionalcodec
