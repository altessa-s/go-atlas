// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package converter provides high-performance struct-to-struct conversion with
// field mapping, filtering, embedded structs, slices, maps, and protocol buffers.
//
// For one-shot conversions use the package-level [Convert] function. For repeated
// conversions with the same options, create a reusable [Converter] via [New] or
// [NewAny] to benefit from cached type metadata and pooled allocations.
//
// Custom type conversions are handled by pluggable codecs registered via
// [WithCodecs]; see the codec sub-packages ([convcodec], [pbwrap], [tspb],
// [unixtime], [jsonpb], [oneof], [mapslice]) for built-in converters.
//
// Lazy iteration over slices and maps is available via [ConvertSeq] and
// [ConvertMapSeq], which convert elements on demand without pre-allocating a
// full destination collection.
//
// Example:
//
//	type User struct{ Name string; Age int }
//	type UserDTO struct{ Name string; Age int }
//	user := User{Name: "John", Age: 30}
//	var dto UserDTO
//	converter.Convert(user, &dto)
package converter
