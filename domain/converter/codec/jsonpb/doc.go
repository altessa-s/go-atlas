// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package jsonpb provides a codec for bidirectional conversion between
// json.RawMessage and structpb.Struct.
//
// Both pointer and non-pointer variants are supported. Nil sources are handled
// gracefully and can be skipped with [WithIgnoreNil].
//
// Example:
//
//	codec := jsonpb.New()
//	converter.Convert(src, &dst, converter.WithCodecs(codec))
package jsonpb
