// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package oneof provides a codec for bidirectional conversion between
// struct-based oneof representations and protobuf oneof interface types.
//
// Conversion directions:
//
//   - Struct with optional pointer fields → protobuf oneof interface wrapper
//   - Protobuf oneof interface wrapper → struct with optional pointer fields
//
// Field matching is case-insensitive. Use [NewForField] to scope the codec
// to a specific field rather than applying it globally.
//
// Example:
//
//	codec := oneof.NewForField("Payload",
//	    oneof.WithWrapperRegistry(map[string]any{
//	        "Contractor": &pb.Invitation_Contractor{},
//	    }),
//	)
package oneof
