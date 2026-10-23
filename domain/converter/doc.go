// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package converter provides high-performance struct-to-struct conversion with
// field mapping, filtering, embedded structs, slices, maps, and protocol buffers.
//
// For one-shot conversions use the package-level [Convert] function; [ConvertE]
// is its error-returning variant for untrusted input and malformed slices. For
// repeated conversions with the same options, create a reusable [Converter] via
// [New] or [NewAny] to benefit from cached type metadata and pooled allocations.
//
// Custom type conversions are handled by pluggable codecs registered via
// [WithCodecs] or error-returning [WithErrorCodecs]; see the codec sub-packages ([convcodec], [pbwrap], [tspb],
// [durpb], [unixtime], [jsonpb], [oneof], [mapslice]) for built-in converters.
//
// Source value handling is controlled by [WithIgnoreZeroValues] (skip every
// zero source value), [WithIgnoreNilValues] (skip nil pointers, slices, and
// maps), and [WithSparseMerge] (apply the source as a sparse partial update
// with JSON Merge Patch-like semantics). [WithSparseMerge] is mutually
// exclusive with [WithIgnoreZeroValues] and subsumes [WithIgnoreNilValues] —
// nil sources are skipped the same way.
//
// [WithIgnoreFields] matches a leaf name at each descended level; [WithIgnorePaths]
// matches an exact dotted source path (promoted fields use their promoted path,
// statically typed map keys are never matched) and is the safer
// choice when the same leaf name appears in several nested objects.
// Interfaces are traversed when an ignore path lies below them; dynamic string
// map keys then become path segments. Nil interfaces and typed nils are preserved.
// A nil interface leaves a concrete destination unchanged; incompatible values
// leave non-empty destination interfaces unchanged. Zero checks apply at field
// boundaries, not again to dereferenced pointer pointees. Sparse merge clears
// present-but-empty dynamic struct pointers to typed nils inside interfaces.
// Traversed containers are detached. Cycle back edges with ignored descendants
// leave destination values unchanged; other back edges retain source references.
// Dynamic slices use fresh zero elements; dynamic arrays merge existing elements.
// Plain overwrite preserves unexported source state in same-type nested structs,
// while ignored exported fields retain destination values. The top-level Convert
// struct retains destination unexported state; root slice elements use nested
// semantics. Sparse merge,
// ignore-zero and ignore-nil modes retain their existing field-wise semantics.
// Error codecs receive full source paths, never statically typed map keys or
// their internal sentinel. A dynamic root map key colliding with that sentinel
// is treated as map-key context and is not exposed to error codecs; legacy
// codecs and built-in conversion still run.
// ConvertE returns all CodecE errors unchanged, including FieldErrors.
// [WithCopySlices] gives assignable slices their own backing array through
// exported struct fields, arrays and map values.
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
