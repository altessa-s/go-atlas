// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package fieldbehavior

import (
	"fmt"
	"slices"

	"github.com/altessa-s/go-atlas/domain/proto/internal/behavior"

	"google.golang.org/genproto/googleapis/api/annotations"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// Strip walks msg and clears every field whose google.api.field_behavior
// annotation intersects the configured behavior set (see [WithBehaviors]).
// With no [WithBehaviors] passed, Strip is a no-op.
//
// Strip traverses nested messages, repeated message elements, and map values
// whose own fields may carry behaviors. When a field itself is annotated, the
// entire subtree is cleared without further descent.
//
// Under [WithStrict] Strip leaves msg untouched and instead returns
// *BehaviorViolationError listing every populated field that would have been
// cleared. With strict disabled (the default), Strip mutates msg in place and
// returns nil unless [WithMaxDepth] is exceeded ([ErrMaxDepthExceeded]).
//
// Message types whose descriptor tree carries no relevant annotation are
// skipped without walking values, so [ErrMaxDepthExceeded] is only reported
// for subtrees that could actually match.
func Strip(msg proto.Message, opts ...Option) error {
	if msg == nil {
		return nil
	}

	o := defaultOptions()
	for _, apply := range opts {
		apply(o)
	}

	if len(o.behaviors) == 0 {
		return nil
	}

	prf := msg.ProtoReflect()

	// Fast path: if no field reachable from this message type carries a
	// relevant behavior, the walk cannot clear or record anything. This is
	// the common case for unannotated responses.
	if !behavior.SubtreeHasAny(prf.Descriptor(), o.behaviors...) {
		return nil
	}

	s := stripper{opts: o}

	if o.strict {
		var violations []BehaviorViolation
		if err := s.walk(prf, "", 0, &violations); err != nil {
			return err
		}

		if len(violations) > 0 {
			return &BehaviorViolationError{Violations: violations}
		}

		return nil
	}

	return s.walk(prf, "", 0, nil)
}

// StripCreate clears fields marked OUTPUT_ONLY or IDENTIFIER. Use it on the
// resource embedded in a Create request before validation. The default set is
// applied first; a caller's [WithBehaviors] overrides it entirely.
func StripCreate(msg proto.Message, opts ...Option) error {
	return Strip(msg, slices.Concat([]Option{WithBehaviors(DefaultCreateBehaviors...)}, opts)...)
}

// StripUpdate clears fields marked OUTPUT_ONLY, IDENTIFIER, or IMMUTABLE.
// Use it on the resource embedded in an Update request before applying the
// update mask.
func StripUpdate(msg proto.Message, opts ...Option) error {
	return Strip(msg, slices.Concat([]Option{WithBehaviors(DefaultUpdateBehaviors...)}, opts)...)
}

// StripResponse clears fields marked INPUT_ONLY. Use it on a server response
// before returning it to the caller so secrets (passwords, tokens) never leak
// through the read path.
func StripResponse(msg proto.Message, opts ...Option) error {
	return Strip(msg, slices.Concat([]Option{WithBehaviors(DefaultResponseBehaviors...)}, opts)...)
}

type stripper struct {
	opts *options
}

// walk visits every field of prf and either clears (mutation mode) or records
// (strict mode) those that match the configured behavior set. violations is
// non-nil only in strict mode.
func (s *stripper) walk(
	prf protoreflect.Message,
	prefix string,
	depth int,
	violations *[]BehaviorViolation,
) error {
	if depth > s.opts.maxDepth {
		return ErrMaxDepthExceeded
	}

	fields := prf.Descriptor().Fields()
	for i := range fields.Len() {
		fd := fields.Get(i)

		if err := s.visit(prf, fd, prefix, depth, violations); err != nil {
			return err
		}
	}

	return nil
}

func (s *stripper) visit(
	prf protoreflect.Message,
	fd protoreflect.FieldDescriptor,
	prefix string,
	depth int,
	violations *[]BehaviorViolation,
) error {
	path := behavior.JoinPath(prefix, string(fd.Name()))

	if behavior.HasAny(fd, s.opts.behaviors...) {
		if !prf.Has(fd) {
			return nil
		}

		if violations != nil {
			*violations = append(*violations, BehaviorViolation{
				Path:     path,
				Behavior: firstMatchingBehavior(fd, s.opts.behaviors),
			})

			return nil
		}

		prf.Clear(fd)

		return nil
	}

	if !prf.Has(fd) {
		return nil
	}

	return s.recurse(prf, fd, path, depth+1, violations)
}

func (s *stripper) recurse(
	prf protoreflect.Message,
	fd protoreflect.FieldDescriptor,
	path string,
	depth int,
	violations *[]BehaviorViolation,
) error {
	// Skip subtrees whose descriptor tree carries no relevant annotation:
	// nothing below can match, so the walk (and Mutable materialization of
	// nested messages) is unnecessary. Message kind covers singular,
	// repeated, and map fields alike.
	if fd.Kind() == protoreflect.MessageKind && !behavior.SubtreeHasAny(fd.Message(), s.opts.behaviors...) {
		return nil
	}

	switch {
	case fd.IsList():
		if fd.Kind() != protoreflect.MessageKind {
			return nil
		}

		return s.walkList(prf, fd, path, depth, violations)

	case fd.IsMap():
		if fd.MapValue().Kind() != protoreflect.MessageKind {
			return nil
		}

		return s.walkMap(prf, fd, path, depth, violations)

	case fd.Kind() == protoreflect.MessageKind:
		nested := s.childMessage(prf, fd, violations)

		return s.walk(nested, path, depth, violations)
	}

	return nil
}

func (s *stripper) walkList(
	prf protoreflect.Message,
	fd protoreflect.FieldDescriptor,
	path string,
	depth int,
	violations *[]BehaviorViolation,
) error {
	list := s.fieldValue(prf, fd, violations).List()

	return behavior.ForEachMessageInList(list, func(i int, elem protoreflect.Message) error {
		return s.walk(elem, fmt.Sprintf("%s[%d]", path, i), depth, violations)
	})
}

func (s *stripper) walkMap(
	prf protoreflect.Message,
	fd protoreflect.FieldDescriptor,
	path string,
	depth int,
	violations *[]BehaviorViolation,
) error {
	mapVal := s.fieldValue(prf, fd, violations).Map()

	var iterErr error

	mapVal.Range(func(k protoreflect.MapKey, v protoreflect.Value) bool {
		childPath := fmt.Sprintf("%s[%q]", path, k.String())
		if err := s.walk(v.Message(), childPath, depth, violations); err != nil {
			iterErr = err
			return false
		}

		return true
	})

	return iterErr
}

// childMessage returns a mutation-capable view of a singular nested message
// in mutate mode, or a read-only view in strict mode.
func (s *stripper) childMessage(
	prf protoreflect.Message,
	fd protoreflect.FieldDescriptor,
	violations *[]BehaviorViolation,
) protoreflect.Message {
	if violations != nil {
		return prf.Get(fd).Message()
	}

	return prf.Mutable(fd).Message()
}

func (s *stripper) fieldValue(
	prf protoreflect.Message,
	fd protoreflect.FieldDescriptor,
	violations *[]BehaviorViolation,
) protoreflect.Value {
	if violations != nil {
		return prf.Get(fd)
	}

	return prf.Mutable(fd)
}

// firstMatchingBehavior returns the first value of behavior.Get(fd) that
// appears in set. It is only called after [behavior.HasAny] returned true, so
// at least one match is guaranteed; a no-match outcome would indicate a
// concurrent descriptor mutation between HasAny and this call, which the
// protobuf runtime does not permit. Panicking surfaces that invariant
// violation immediately instead of attributing a bogus
// FIELD_BEHAVIOR_UNSPECIFIED to a real violation downstream.
func firstMatchingBehavior(
	fd protoreflect.FieldDescriptor,
	set []annotations.FieldBehavior,
) annotations.FieldBehavior {
	for _, got := range behavior.Get(fd) {
		if slices.Contains(set, got) {
			return got
		}
	}

	panic("fieldbehavior: firstMatchingBehavior invoked without a matching behavior — HasAny invariant violated")
}
