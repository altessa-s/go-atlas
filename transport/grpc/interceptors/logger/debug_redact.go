// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package logger

import (
	"sync"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"
)

// DebugRedact is a [PayloadRedactor] honoring the standard debug_redact field option ([descriptorpb.FieldOptions.DebugRedact]): it
// returns a copy of m without the values of the fields marked debug_redact, at any depth (nested messages, lists and map values), or
// m itself when its type cannot carry such a field. m is never changed. Whether a message type can carry a redacted field is computed
// once per message descriptor and cached.
//
// A google.protobuf.Any may hold any message, so a type containing an Any anywhere always counts as able to carry one. The payload
// of every Any is resolved through [protoregistry.GlobalTypes]: when its type can carry a redacted field it is unpacked, redacted
// and repacked (deterministic marshal) in the copy. A payload that cannot be checked — an unresolvable type URL, or bytes that do not
// unmarshal — fails closed: the Any is replaced by an empty google.protobuf.Empty (type URL and bytes both dropped). Every resolved
// payload is decoded to check it, and the unpacking of one logged message is bounded (16 nested Any layers, 1 MiB of decoded payload
// bytes in total); a payload beyond the bounds is replaced by the same placeholder.
//
// [WithDebugRedact] applies it before the [WithPayloadRedactor] redactor, if any.
func DebugRedact(m proto.Message) any {
	return redactDebugFields(m)
}

// redactCache maps a message descriptor to whether its type can carry a debug_redact field (directly or in a nested message type).
// Descriptors are immutable, so the answer never changes; keys are descriptor identities, so two descriptors that share a full name
// (dynamic messages) never share an entry.
var redactCache sync.Map // protoreflect.MessageDescriptor -> bool

// redactDebugFields implements [DebugRedact].
func redactDebugFields(m proto.Message) proto.Message {
	if m == nil || !canCarryRedacted(m.ProtoReflect().Descriptor()) {
		return m
	}
	c := proto.Clone(m)
	clearRedacted(c.ProtoReflect())
	return c
}

// canCarryRedacted reports, through the cache, whether a message type can carry a debug_redact field.
func canCarryRedacted(md protoreflect.MessageDescriptor) bool {
	if v, ok := redactCache.Load(md); ok {
		has, _ := v.(bool) // the cache only holds bools
		return has
	}
	has := hasRedacted(md, map[protoreflect.FullName]bool{})
	redactCache.Store(md, has)
	return has
}

// isRedacted reports a field marked debug_redact.
func isRedacted(fd protoreflect.FieldDescriptor) bool {
	opts, ok := fd.Options().(*descriptorpb.FieldOptions)
	return ok && opts.GetDebugRedact()
}

// anyName is the full name of google.protobuf.Any, whose payload is checked at run time.
const anyName protoreflect.FullName = "google.protobuf.Any"

// emptyAnyURL is the type URL of the placeholder that replaces an Any whose payload cannot be checked.
const emptyAnyURL = "type.googleapis.com/google.protobuf.Empty"

// hasRedacted reports whether a message type can carry a redacted field, directly, in a nested message type or in an Any; seen
// breaks cycles of recursive types.
func hasRedacted(md protoreflect.MessageDescriptor, seen map[protoreflect.FullName]bool) bool {
	if md.FullName() == anyName {
		return true
	}
	if seen[md.FullName()] {
		return false
	}
	seen[md.FullName()] = true
	fields := md.Fields()
	for i := range fields.Len() {
		fd := fields.Get(i)
		if isRedacted(fd) {
			return true
		}
		if fd.Message() != nil && hasRedacted(fd.Message(), seen) {
			return true
		}
	}
	return false
}

// Limits of the Any unpacking done for one logged message. A payload beyond them is replaced by the empty placeholder, so a crafted
// message (thousands of Any layers, or many large payloads) costs bounded work during logging; the protobuf decoder's own recursion
// limit restarts with every unpacked payload and cannot bound this.
const (
	// maxAnyDepth bounds how many Any layers are unpacked inside one another.
	maxAnyDepth = 16
	// maxAnyDecodedBytes bounds the payload bytes decoded across all Any values of one logged message.
	maxAnyDecodedBytes = 1 << 20
)

// redactWalk carries the shared Any limits through the redaction of one logged message.
type redactWalk struct {
	anyDepth int
	decoded  int
}

// clearRedacted removes the redacted fields of m and of its nested messages, including the payloads of Any values.
func clearRedacted(m protoreflect.Message) {
	new(redactWalk).clear(m)
}

func (w *redactWalk) clear(m protoreflect.Message) {
	if m.Descriptor().FullName() == anyName {
		w.redactAny(m)
		return
	}
	m.Range(func(fd protoreflect.FieldDescriptor, v protoreflect.Value) bool {
		switch {
		case isRedacted(fd):
			m.Clear(fd)
		case fd.IsList() && fd.Message() != nil:
			l := v.List()
			for i := range l.Len() {
				w.clear(l.Get(i).Message())
			}
		case fd.IsMap() && fd.MapValue().Message() != nil:
			v.Map().Range(func(_ protoreflect.MapKey, mv protoreflect.Value) bool {
				w.clear(mv.Message())
				return true
			})
		case fd.Message() != nil && !fd.IsList() && !fd.IsMap():
			w.clear(v.Message())
		}
		return true
	})
}

// redactAny redacts the payload of the Any m in place (m belongs to the copy being logged). Every resolved payload is decoded first,
// within the shared limits: a payload whose type cannot carry a redacted field keeps its original bytes once it decodes; one whose
// type can is redacted and repacked. A payload that cannot be resolved, does not decode, or exceeds the limits is replaced by the
// empty placeholder.
func (w *redactWalk) redactAny(m protoreflect.Message) {
	fields := m.Descriptor().Fields()
	urlField, valueField := fields.ByNumber(1), fields.ByNumber(2) //nolint:mnd // google.protobuf.Any field numbers
	if urlField == nil || valueField == nil {
		return
	}
	url := m.Get(urlField).String()
	value := m.Get(valueField).Bytes()
	if url == "" && len(value) == 0 {
		return // an empty Any carries nothing
	}
	if w.anyDepth >= maxAnyDepth || w.decoded+len(value) > maxAnyDecodedBytes {
		replaceAny(m, urlField, valueField)
		return
	}
	w.decoded += len(value)
	mt, err := protoregistry.GlobalTypes.FindMessageByURL(url)
	if err != nil {
		replaceAny(m, urlField, valueField)
		return
	}
	inner := mt.New().Interface()
	if err = (proto.UnmarshalOptions{Resolver: protoregistry.GlobalTypes}).Unmarshal(value, inner); err != nil {
		replaceAny(m, urlField, valueField)
		return
	}
	if !canCarryRedacted(mt.Descriptor()) {
		return // decoded fine and holds no secret: keep the original bytes
	}
	w.anyDepth++
	w.clear(inner.ProtoReflect())
	w.anyDepth--
	packed, err := proto.MarshalOptions{Deterministic: true}.Marshal(inner)
	if err != nil {
		replaceAny(m, urlField, valueField)
		return
	}
	m.Set(valueField, protoreflect.ValueOfBytes(packed))
}

// replaceAny turns the Any m into the empty placeholder: fail closed for a payload that cannot be checked.
func replaceAny(m protoreflect.Message, urlField, valueField protoreflect.FieldDescriptor) {
	m.Set(urlField, protoreflect.ValueOfString(emptyAnyURL))
	m.Clear(valueField)
}
