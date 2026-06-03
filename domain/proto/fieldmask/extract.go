// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package fieldmask

import (
	"errors"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

const (
	// DefaultUpdateMaskFieldName is the AIP-134 conventional name for the
	// update mask field on an Update* request message.
	DefaultUpdateMaskFieldName = "update_mask"

	// DefaultReadMaskFieldName is the AIP-157 conventional name for the
	// read mask field on a Get/List/Search request message.
	DefaultReadMaskFieldName = "read_mask"

	// fieldMaskMessageFullName is the protobuf full name of the wire-level
	// field mask message.
	fieldMaskMessageFullName protoreflect.FullName = "google.protobuf.FieldMask"
)

// ErrFieldNotSettable is returned by [SetUpdateMask] when the request message
// does not expose a settable update_mask field of type google.protobuf.FieldMask.
var ErrFieldNotSettable = errors.New("fieldmask: update_mask field is not present or has a wrong type")

// extractOptions configures the lookup performed by [ExtractUpdateMask],
// [ExtractReadMask], and [SetUpdateMask].
type extractOptions struct {
	maskField     string
	resourceField string
}

// ExtractOption tunes the field-name lookup performed by [ExtractUpdateMask],
// [ExtractReadMask], and [SetUpdateMask]. The zero options use the AIP-134 /
// AIP-157 defaults (update_mask, read_mask) and pick the first non-mask
// message field as the resource carrier.
type ExtractOption func(*extractOptions)

// WithMaskField overrides the conventional update_mask / read_mask field name
// used when extracting or writing back the mask. The match is by descriptor
// field name (proto field, not Go field).
func WithMaskField(name string) ExtractOption {
	return func(o *extractOptions) {
		if name != "" {
			o.maskField = name
		}
	}
}

// WithResourceField overrides the resource carrier field name used by
// [ExtractUpdateMask] to locate the sibling sub-message updated by the mask.
// When unset the first non-mask message field is selected.
func WithResourceField(name string) ExtractOption {
	return func(o *extractOptions) {
		if name != "" {
			o.resourceField = name
		}
	}
}

func (o *extractOptions) maskOr(def string) string {
	if o.maskField != "" {
		return o.maskField
	}

	return def
}

func newExtractOptions(opts []ExtractOption) *extractOptions {
	o := &extractOptions{}
	for _, opt := range opts {
		opt(o)
	}

	return o
}

// ExtractUpdateMask locates the AIP-134 update_mask field on req and the
// sibling resource sub-message updated by that mask. It returns (mask,
// resource, true) when both are present and the message shape matches the
// convention.
//
// Selection rules:
//
//   - The mask field is the [fieldmaskpb.FieldMask] field named "update_mask"
//     (or the override from [WithMaskField]).
//   - The resource is the field named via [WithResourceField], or, if no
//     override is set, the first non-mask message field declared on req.
//     Repeated and map message fields are skipped.
//   - Both fields must be populated on req; an unset mask or resource yields
//     ok=false so the caller can fall through to a passthrough.
func ExtractUpdateMask(req proto.Message, opts ...ExtractOption) (mask *fieldmaskpb.FieldMask, resource proto.Message, ok bool) {
	o := newExtractOptions(opts)

	maskMsg, prf, maskFD, found := lookupFieldMask(req, o.maskOr(DefaultUpdateMaskFieldName), true)
	if !found || maskMsg == nil {
		return nil, nil, false
	}

	resFD := resolveResourceField(prf.Descriptor().Fields(), maskFD, o.resourceField)
	if resFD == nil || !prf.Has(resFD) {
		return maskMsg, nil, false
	}

	return maskMsg, prf.Get(resFD).Message().Interface(), true
}

// ExtractReadMask locates the AIP-157 read_mask field on req. Returns
// (mask, true) when the field is present and populated.
func ExtractReadMask(req proto.Message, opts ...ExtractOption) (mask *fieldmaskpb.FieldMask, ok bool) {
	o := newExtractOptions(opts)

	maskMsg, _, _, found := lookupFieldMask(req, o.maskOr(DefaultReadMaskFieldName), true)
	if !found || maskMsg == nil {
		return nil, false
	}

	return maskMsg, true
}

// SetUpdateMask writes mask back onto req's update_mask field. Used by the
// fieldmask gRPC interceptor after [FieldMask.ApplyUpdateMask] removes
// OUTPUT_ONLY entries so the handler sees a coherent (mask, resource) pair.
// Returns [ErrFieldNotSettable] when req does not expose an update_mask
// field of type [fieldmaskpb.FieldMask].
func SetUpdateMask(req proto.Message, mask *fieldmaskpb.FieldMask, opts ...ExtractOption) error {
	o := newExtractOptions(opts)

	_, prf, fd, found := lookupFieldMask(req, o.maskOr(DefaultUpdateMaskFieldName), false)
	if !found {
		return ErrFieldNotSettable
	}

	if mask == nil {
		prf.Clear(fd)
		return nil
	}

	prf.Set(fd, protoreflect.ValueOfMessage(mask.ProtoReflect()))

	return nil
}

// lookupFieldMask resolves the named [fieldmaskpb.FieldMask] field on req. It
// is the single point of validation shared by [ExtractUpdateMask],
// [ExtractReadMask], and [SetUpdateMask] so the "find the well-known mask
// field" boilerplate stays consistent across read, update, and writeback
// paths.
//
// Returns found=true only when req is non-nil, reflects to a valid message,
// and exposes a singular google.protobuf.FieldMask field named name. When
// mustBeSet is true the field must additionally be populated on req — used
// by the extractor paths so an unset mask yields ok=false; SetUpdateMask
// passes mustBeSet=false because it needs the descriptor even to clear the
// field. The returned mask is non-nil only when the field is populated and
// the underlying message value type-asserts cleanly.
func lookupFieldMask(req proto.Message, name string, mustBeSet bool) (
	mask *fieldmaskpb.FieldMask,
	prf protoreflect.Message,
	fd protoreflect.FieldDescriptor,
	found bool,
) {
	if req == nil {
		return nil, nil, nil, false
	}

	prf = req.ProtoReflect()
	if !prf.IsValid() {
		return nil, nil, nil, false
	}

	fd = prf.Descriptor().Fields().ByName(protoreflect.Name(name))
	if !isFieldMask(fd) {
		return nil, nil, nil, false
	}

	if !prf.Has(fd) {
		if mustBeSet {
			return nil, nil, nil, false
		}
		return nil, prf, fd, true
	}

	mask, _ = prf.Get(fd).Message().Interface().(*fieldmaskpb.FieldMask)
	return mask, prf, fd, true
}

// isFieldMask reports whether fd is a singular message field whose descriptor
// is google.protobuf.FieldMask.
func isFieldMask(fd protoreflect.FieldDescriptor) bool {
	if fd == nil || fd.Kind() != protoreflect.MessageKind || fd.IsList() || fd.IsMap() {
		return false
	}

	md := fd.Message()
	if md == nil {
		return false
	}

	return md.FullName() == fieldMaskMessageFullName
}

// resolveResourceField picks the resource carrier field. When override is
// non-empty the descriptor is looked up by name; otherwise the first
// non-mask, non-repeated, non-map message field is returned.
func resolveResourceField(fields protoreflect.FieldDescriptors, maskFD protoreflect.FieldDescriptor, override string) protoreflect.FieldDescriptor {
	if override != "" {
		fd := fields.ByName(protoreflect.Name(override))
		if fd == nil || fd.Kind() != protoreflect.MessageKind || fd.IsList() || fd.IsMap() {
			return nil
		}

		return fd
	}

	for i := range fields.Len() {
		fd := fields.Get(i)
		if fd == maskFD {
			continue
		}

		if fd.Kind() != protoreflect.MessageKind || fd.IsList() || fd.IsMap() {
			continue
		}

		if fd.Message() != nil && fd.Message().FullName() == fieldMaskMessageFullName {
			continue
		}

		return fd
	}

	return nil
}
