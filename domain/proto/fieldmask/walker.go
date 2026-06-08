// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package fieldmask

import (
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/structpb"
)

// maskOp carries the per-decision callbacks that customize how
// [FieldMask.walk] reacts to each field of a message.
//
// [FieldMask.iterateFilter] and [FieldMask.iteratePrune] are mirror operations
// over the same traversal state machine — they share ProtoReflect Range,
// list/map dispatch, AIP-161 wildcard semantics, and dynamic well-known type
// short-circuiting — and differ only in what "matched" and "unmatched" mean.
// maskOp expresses that delta as a handful of callbacks instead of two
// near-identical 90-line functions.
//
// All callbacks are required; the walker does not check for nil. WKT
// dispatchers (onStruct/onListValue/onStructValue) receive the same FieldMask
// the walker was invoked with so they can recurse through the structpb
// helpers ([FieldMask.filterStruct] / pruneStruct / …).
type maskOp struct {
	// onUnmatched fires for a field that is not present in the mask at this
	// level. iterateFilter clears the field; iteratePrune is a no-op.
	onUnmatched func(prf protoreflect.Message, fd protoreflect.FieldDescriptor)

	// onLeaf fires for a field that is present in the mask with a nil
	// sub-mask. iterateFilter keeps it; iteratePrune clears it.
	onLeaf func(prf protoreflect.Message, fd protoreflect.FieldDescriptor)

	// recurse dispatches into a nested message for a branch entry in the
	// mask. iterateFilter recurses with iterateFilter; iteratePrune with
	// iteratePrune.
	recurse func(nested FieldMask, msg proto.Message)

	// onListWildcardLeaf fires when the sub-mask for a list field is
	// {"*": nil}. iterateFilter keeps the entire list; iteratePrune
	// clears it.
	onListWildcardLeaf func(prf protoreflect.Message, fd protoreflect.FieldDescriptor)

	// onMapEntryUnmatched fires for a map entry whose key is not targeted by
	// either a specific rule or a "*" wildcard. iterateFilter drops the
	// entry; iteratePrune keeps it.
	onMapEntryUnmatched func(mm protoreflect.Map, k protoreflect.MapKey)

	// onMapEntryLeaf fires for a map entry whose matching sub-mask is nil
	// (leaf rule). iterateFilter keeps it; iteratePrune drops it.
	onMapEntryLeaf func(mm protoreflect.Map, k protoreflect.MapKey)

	// onStruct / onListValue / onStructValue dispatch the mask into the
	// structpb walkers when msg is the corresponding dynamic well-known type.
	onStruct      func(msk FieldMask, s *structpb.Struct)
	onListValue   func(msk FieldMask, lv *structpb.ListValue)
	onStructValue func(msk FieldMask, v *structpb.Value)
}

// walk iterates over the populated fields of msg, dispatching to op for each
// field based on whether it is matched, unmatched, or a leaf in the mask.
// Dynamic well-known types are forwarded to the WKT dispatchers carried in op.
func (msk FieldMask) walk(msg proto.Message, op maskOp) {
	if s, ok := msg.(*structpb.Struct); ok {
		op.onStruct(msk, s)
		return
	}
	if lv, ok := msg.(*structpb.ListValue); ok {
		op.onListValue(msk, lv)
		return
	}
	if v, ok := msg.(*structpb.Value); ok {
		op.onStructValue(msk, v)
		return
	}

	prf := msg.ProtoReflect()
	prf.Range(func(fd protoreflect.FieldDescriptor, _ protoreflect.Value) bool {
		nested, exists := msk[string(fd.Name())]
		if !exists {
			op.onUnmatched(prf, fd)
			return true
		}
		if nested == nil {
			op.onLeaf(prf, fd)
			return true
		}

		switch {
		case fd.IsList():
			perElement := nested
			if wildcardMask, ok := nested[WildcardSegment]; ok {
				if wildcardMask == nil {
					op.onListWildcardLeaf(prf, fd)
					return true
				}
				perElement = wildcardMask
			}
			applyToMessageList(fd, prf.Get(fd).List(), perElement, op.recurse)

		case fd.IsMap():
			mm := prf.Get(fd).Map()
			if !mm.IsValid() || mm.Len() == 0 {
				return true
			}
			wildcardMask, hasWildcard := nested[WildcardSegment]
			mapValueDesc := fd.MapValue()
			mm.Range(func(k protoreflect.MapKey, mapVal protoreflect.Value) bool {
				specificMask, hasSpecific := nested[k.String()]
				var finalMask FieldMask
				switch {
				case hasSpecific:
					finalMask = specificMask
				case hasWildcard:
					finalMask = wildcardMask
				default:
					op.onMapEntryUnmatched(mm, k)
					return true
				}
				if finalMask == nil {
					op.onMapEntryLeaf(mm, k)
					return true
				}
				if mapValueDesc != nil && mapValueDesc.Kind() == protoreflect.MessageKind {
					op.recurse(finalMask, mapVal.Message().Interface())
				}
				return true
			})

		case fd.Kind() == protoreflect.MessageKind:
			op.recurse(nested, prf.Get(fd).Message().Interface())
		}
		return true
	})
}

// structOp carries the per-key callbacks for [FieldMask.walkStruct].
// filterStruct and pruneStruct mirror each other on a google.protobuf.Struct
// the same way iterateFilter and iteratePrune mirror each other on a
// protobuf message.
type structOp struct {
	// onUnmatched fires for a struct key not present in the mask.
	// filterStruct deletes it; pruneStruct keeps it.
	onUnmatched func(s *structpb.Struct, key string)
	// onLeaf fires for a struct key present in the mask with a nil
	// sub-mask. filterStruct keeps it; pruneStruct deletes it.
	onLeaf func(s *structpb.Struct, key string)
	// recurse dispatches into the value for a branch entry.
	recurse func(nested FieldMask, v *structpb.Value)
}

// walkStruct iterates over s.Fields, dispatching to op for each key. Empty or
// nil structs are a no-op.
func (msk FieldMask) walkStruct(s *structpb.Struct, op structOp) {
	if s == nil || len(s.GetFields()) == 0 {
		return
	}
	for key, val := range s.GetFields() {
		nested, exists := msk[key]
		if !exists {
			op.onUnmatched(s, key)
			continue
		}
		if nested == nil {
			op.onLeaf(s, key)
			continue
		}
		if val != nil {
			op.recurse(nested, val)
		}
	}
}

// dispatchStructValue invokes structFn for a struct-valued Value, listFn for
// a list-valued Value, and is a no-op for scalar or nil Values. The single
// dispatcher is shared by filter, prune, and path-enumeration walkers so the
// "is it a Struct or a ListValue?" branch is written once.
func dispatchStructValue(v *structpb.Value, structFn func(*structpb.Struct), listFn func(*structpb.ListValue)) {
	if v == nil {
		return
	}
	if sv := v.GetStructValue(); sv != nil {
		structFn(sv)
		return
	}
	if lv := v.GetListValue(); lv != nil {
		listFn(lv)
	}
}

// walkListValue invokes apply for each non-nil element of lv. Nil lv is a
// no-op. Used by filterListValue and pruneListValue to apply the same mask
// to every Value in the list.
func walkListValue(lv *structpb.ListValue, apply func(*structpb.Value)) {
	if lv == nil {
		return
	}
	for _, v := range lv.GetValues() {
		if v != nil {
			apply(v)
		}
	}
}
