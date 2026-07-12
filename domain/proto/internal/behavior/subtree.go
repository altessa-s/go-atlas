// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package behavior

import (
	"sync"

	"google.golang.org/genproto/googleapis/api/annotations"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// behaviorMask is a bitset of annotations.FieldBehavior values (bit i set
// means behavior i is present). The enum tops out at IDENTIFIER (8), so a
// uint32 leaves ample headroom.
type behaviorMask uint32

// maskOf folds a behavior set into a bitmask. Values outside the bitset
// range are ignored — they cannot occur for the generated enum.
func maskOf(set []annotations.FieldBehavior) behaviorMask {
	var m behaviorMask
	for _, b := range set {
		if b >= 0 && b < 32 {
			m |= 1 << uint(b)
		}
	}
	return m
}

// subtreeCache memoizes the union of field_behavior annotations reachable
// from a message descriptor, keyed by descriptor identity. One DFS per
// message type per process; lookups afterwards are a single Load.
var subtreeCache sync.Map // protoreflect.MessageDescriptor → behaviorMask

// SubtreeHasAny reports whether any field reachable from md — its own fields
// and, transitively, the fields of every nested, repeated, or map-value
// message type — carries one of the given behaviors. It answers from the
// descriptor tree only, so a false result guarantees that walking a message
// of this type can never match, regardless of populated values.
func SubtreeHasAny(md protoreflect.MessageDescriptor, set ...annotations.FieldBehavior) bool {
	if len(set) == 0 {
		return false
	}
	return subtreeMask(md)&maskOf(set) != 0
}

func subtreeMask(md protoreflect.MessageDescriptor) behaviorMask {
	if v, ok := subtreeCache.Load(md); ok {
		return v.(behaviorMask) //nolint:errcheck // type guaranteed by Store
	}

	m := computeSubtreeMask(md, map[protoreflect.FullName]bool{})
	subtreeCache.Store(md, m)

	return m
}

// computeSubtreeMask unions the annotations of md's transitive field tree.
// visiting guards against recursive message types; revisited nodes
// contribute nothing extra because their fields are already accumulated in
// this traversal.
func computeSubtreeMask(md protoreflect.MessageDescriptor, visiting map[protoreflect.FullName]bool) behaviorMask {
	if visiting[md.FullName()] {
		return 0
	}
	visiting[md.FullName()] = true

	var m behaviorMask
	fields := md.Fields()
	for i := range fields.Len() {
		fd := fields.Get(i)
		m |= maskOf(Get(fd))

		// Message-kind covers singular, repeated, and map fields alike: for
		// maps fd.Message() is the synthetic entry type whose value field
		// leads to the real message.
		if fd.Kind() == protoreflect.MessageKind || fd.Kind() == protoreflect.GroupKind {
			m |= computeSubtreeMask(fd.Message(), visiting)
		}
	}

	return m
}
