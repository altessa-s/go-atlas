// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package prometheus

// GetMessageSize extracts the serialized byte size of msg by probing for
// known sizing interfaces in the following order:
//
//  1. ProtoSize() int   -- gogoproto-generated messages
//  2. Size() int        -- standard google.golang.org/protobuf or custom types
//  3. XXX_Size() int    -- legacy proto2/proto3 generated code
//
// Returns (0, false) when msg is nil or implements none of these.
func GetMessageSize(msg any) (int, bool) {
	if msg == nil {
		return 0, false
	}

	// Try to get size from proto message (gogoproto style)
	if protoMsg, ok := msg.(interface{ ProtoSize() int }); ok {
		return protoMsg.ProtoSize(), true
	}

	// Try alternative size method (standard proto or custom)
	if sizeable, ok := msg.(interface{ Size() int }); ok {
		return sizeable.Size(), true
	}

	// Try XXX_Size method for older proto implementations
	if legacyMsg, ok := msg.(interface{ XXX_Size() int }); ok {
		return legacyMsg.XXX_Size(), true
	}

	return 0, false
}

// Sizer is satisfied by any type that can report its serialized size in
// bytes. Used by metrics recording to track message payload sizes.
type Sizer interface {
	Size() int
}

// ProtoSizer is the gogoproto-style sizing interface. It is checked before
// [Sizer] in [GetMessageSize] because gogoproto types often implement both.
type ProtoSizer interface {
	ProtoSize() int
}
