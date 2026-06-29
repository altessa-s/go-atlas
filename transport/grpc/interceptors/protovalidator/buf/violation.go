// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package bufhelpers

import (
	"strings"

	"buf.build/gen/go/bufbuild/protovalidate/protocolbuffers/go/buf/validate"

	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/protovalidator/reasoncode"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
)

var _ reasoncode.Violation = violationView{}

// violationView adapts a protovalidate violation and the validated message to
// [reasoncode.Violation], so reasoncode can derive a reason code without
// depending on protovalidate. The numeric leaf field is resolved once up front
// and reused by NumericValue and LowerBound.
type violationView struct {
	ruleID string
	field  *validate.FieldPath

	leafValue float64
	leafField protoreflect.FieldDescriptor
	leafOK    bool
}

// newViolationView resolves the numeric leaf field once. msg may be nil, in which
// case numeric context is unavailable and NumericValue/LowerBound report ok=false.
func newViolationView(ruleID string, field *validate.FieldPath, msg proto.Message) violationView {
	view := violationView{ruleID: ruleID, field: field}
	if msg != nil && field != nil {
		view.leafValue, view.leafField, view.leafOK = leafNumericValue(msg.ProtoReflect(), field.GetElements())
	}
	return view
}

// RuleID implements [reasoncode.Violation].
func (v violationView) RuleID() string { return v.ruleID }

// FieldName implements [reasoncode.Violation], returning the last field-path segment.
func (v violationView) FieldName() string {
	elements := v.field.GetElements()
	if len(elements) == 0 {
		return ""
	}
	return elements[len(elements)-1].GetFieldName()
}

// NumericValue implements [reasoncode.Violation].
func (v violationView) NumericValue() (float64, bool) {
	return v.leafValue, v.leafOK
}

// LowerBound implements [reasoncode.Violation].
func (v violationView) LowerBound() (float64, bool, bool) {
	if !v.leafOK {
		return 0, false, false
	}
	return fieldLowerBound(v.leafField, v.ruleID)
}

// leafNumericValue walks a field path on m and returns the leaf field's value as a
// float64 together with its descriptor. ok is false if the path does not resolve
// to a numeric scalar.
func leafNumericValue(m protoreflect.Message, elements []*validate.FieldPathElement) (float64, protoreflect.FieldDescriptor, bool) {
	current := m
	for i, element := range elements {
		fd := current.Descriptor().Fields().ByNumber(protoreflect.FieldNumber(element.GetFieldNumber()))
		if fd == nil {
			return 0, nil, false
		}
		if i == len(elements)-1 {
			value, ok := numericFloat(fd, current.Get(fd))
			return value, fd, ok
		}
		if fd.Kind() != protoreflect.MessageKind || fd.IsList() || fd.IsMap() {
			return 0, nil, false
		}
		current = current.Get(fd).Message()
	}
	return 0, nil, false
}

// numericFloat converts a scalar numeric protoreflect value to a float64.
func numericFloat(fd protoreflect.FieldDescriptor, v protoreflect.Value) (float64, bool) {
	switch fd.Kind() {
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind,
		protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
		return float64(v.Int()), true
	case protoreflect.Uint32Kind, protoreflect.Fixed32Kind,
		protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		return float64(v.Uint()), true
	case protoreflect.FloatKind, protoreflect.DoubleKind:
		return v.Float(), true
	default:
		return 0, false
	}
}

// fieldLowerBound reads a field's lower numeric bound (gte or gt, per the rule ID)
// from its buf.validate rules, and reports whether it is inclusive (gte).
func fieldLowerBound(fd protoreflect.FieldDescriptor, ruleID string) (float64, bool, bool) {
	opts, ok := fd.Options().(*descriptorpb.FieldOptions)
	if !ok || opts == nil || !proto.HasExtension(opts, validate.E_Field) {
		return 0, false, false
	}
	rules, ok := proto.GetExtension(opts, validate.E_Field).(*validate.FieldRules)
	if !ok || rules == nil {
		return 0, false, false
	}

	inclusive := strings.Contains(ruleID, ".gte")
	switch {
	case rules.GetInt64() != nil:
		if inclusive {
			return float64(rules.GetInt64().GetGte()), true, true
		}
		return float64(rules.GetInt64().GetGt()), false, true
	case rules.GetInt32() != nil:
		if inclusive {
			return float64(rules.GetInt32().GetGte()), true, true
		}
		return float64(rules.GetInt32().GetGt()), false, true
	case rules.GetUint64() != nil:
		if inclusive {
			return float64(rules.GetUint64().GetGte()), true, true
		}
		return float64(rules.GetUint64().GetGt()), false, true
	case rules.GetDouble() != nil:
		if inclusive {
			return rules.GetDouble().GetGte(), true, true
		}
		return rules.GetDouble().GetGt(), false, true
	case rules.GetFloat() != nil:
		if inclusive {
			return float64(rules.GetFloat().GetGte()), true, true
		}
		return float64(rules.GetFloat().GetGt()), false, true
	default:
		return 0, false, false
	}
}
