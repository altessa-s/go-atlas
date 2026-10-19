// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package fieldmask

import (
	"github.com/altessa-s/go-atlas/domain/proto/internal/behavior"

	"google.golang.org/genproto/googleapis/api/annotations"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// FromSetFieldsForUpdate returns the update mask an update request implies
// when its update_mask is empty (AIP-134: update every populated field), ready
// for [FieldMask.ApplyUpdateMask]. It is [FromSetFields] of msg with the
// server-managed fields skipped (see [WithSkipFields]):
//   - every IDENTIFIER field, top-level and in populated singular
//     sub-messages: the identifier addresses the resource (AIP-203), it is
//     not a field the update changes;
//   - every OUTPUT_ONLY field of a populated sub-message: never written by
//     an update (a top-level one is dropped by ApplyUpdateMask);
//   - the unset IMMUTABLE fields of a populated sub-message;
//   - the absent singular sub-messages of a populated one whose schema holds
//     a server-managed field (OUTPUT_ONLY, IDENTIFIER or IMMUTABLE) at any
//     depth.
//
// A nested skip drops the parents that cover it, so a populated sub-message
// holding such a field is masked by its other set fields instead of whole,
// which ApplyUpdateMask rejects. A sub-message whose only populated field is
// skipped drops out of the mask altogether. A set nested IMMUTABLE field is
// not skipped: it stays in the mask and ApplyUpdateMask rejects it like a
// top-level one. Repeated and map fields are named whole and replaced as a
// whole, as with [FromSetFields].
//
// The result can be empty. Pass it to ApplyUpdateMask all the same: the empty
// mask authorizes nothing and the filter clears the skipped client values.
func FromSetFieldsForUpdate(msg proto.Message) FieldMask {
	if msg == nil {
		return FieldMask{}
	}
	return FromSetFields[FieldMask](msg, WithSkipFields(synthesisSkipFields(msg.ProtoReflect())...))
}

// synthesisSkipFields returns the skip paths of [FromSetFieldsForUpdate] for
// msg. Lists and maps are not descended: [FromSetFields] names them whole.
// The walk follows the populated value rather than the descriptor, so a
// recursive message type terminates at its last populated level and every
// identifier on the chain is found.
func synthesisSkipFields(msg protoreflect.Message) []string {
	var paths []string
	collectSkipFields(msg, "", &paths)
	return paths
}

// collectSkipFields appends the skip paths of msg under prefix (see
// [synthesisSkipFields]).
func collectSkipFields(msg protoreflect.Message, prefix string, paths *[]string) {
	fields := msg.Descriptor().Fields()
	for i := range fields.Len() {
		fd := fields.Get(i)
		path := behavior.JoinPath(prefix, string(fd.Name()))
		switch serverManagedBehavior(fd) {
		case annotations.FieldBehavior_OUTPUT_ONLY:
			// Never written by the update: a nested one is skipped so its
			// parent is masked by its other set fields, a top-level one is
			// dropped by ApplyUpdateMask. Not descended.
			if prefix != "" {
				*paths = append(*paths, path)
			}
			continue
		case annotations.FieldBehavior_IDENTIFIER:
			*paths = append(*paths, path)
			continue
		case annotations.FieldBehavior_IMMUTABLE:
			if prefix != "" && !msg.Has(fd) {
				*paths = append(*paths, path)
				continue
			}
		default:
		}
		switch {
		case !isExpandableMessage(fd):
		case msg.Has(fd):
			collectSkipFields(msg.Get(fd).Message(), path, paths)
		case prefix != "" && hasProtectedDescendants(fd.Message(), map[protoreflect.FullName]bool{}):
			// An absent sub-message whose schema holds a protected field:
			// skipping it keeps its populated parent from being selected
			// whole, which ApplyUpdateMask checks against the schema.
			*paths = append(*paths, path)
		}
	}
}

// DropIdentifiers removes the IDENTIFIER entries of msg's schema from the
// mask, descending into the sub-masks of singular message fields and into the
// element sub-masks of repeated and map fields. A branch left empty is removed
// too: it names no field.
//
// [FieldMask.ApplyUpdateMask] adds the set IDENTIFIER fields to the mask so
// they survive the filter; they address the resource (AIP-203) rather than
// name fields to change, so a caller that hands the cleaned mask on — the gRPC
// fieldmask interceptor writes it back to the request — drops them first. An
// IDENTIFIER the client names in the mask is rejected by ApplyUpdateMask, so
// every entry removed then was added by it.
//
// Mutates the receiver.
func (msk FieldMask) DropIdentifiers(msg proto.Message) {
	if msg == nil {
		return
	}
	msk.dropIdentifiers(msg.ProtoReflect().Descriptor())
}

// dropIdentifiers is [FieldMask.DropIdentifiers] over a descriptor. Keys md
// does not declare ("*", map keys, indexes) are left alone.
func (msk FieldMask) dropIdentifiers(md protoreflect.MessageDescriptor) {
	fields := md.Fields()
	for name, nested := range msk {
		fd := fields.ByName(protoreflect.Name(name))
		switch {
		case fd == nil:
			continue
		case behavior.Has(fd, annotations.FieldBehavior_IDENTIFIER):
			delete(msk, name)
			continue
		case nested == nil:
			continue
		case isExpandableMessage(fd):
			nested.dropIdentifiers(fd.Message())
		case fd.IsList() || fd.IsMap():
			elem := wildcardElementDescriptor(fd)
			if elem == nil {
				continue
			}
			for key, sub := range elementMasks(fd, nested) {
				sub.dropIdentifiers(elem)
				if key != "" && len(sub) == 0 {
					delete(nested, key)
				}
			}
		}
		if len(nested) == 0 {
			delete(msk, name)
		}
	}
}
