// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package bufhelpers

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"buf.build/gen/go/bufbuild/protovalidate/protocolbuffers/go/buf/validate"
	"buf.build/go/protovalidate"

	"github.com/altessa-s/proto-gen-go/badrequest/v1"

	"github.com/altessa-s/go-atlas/core/types/ptr"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
)

// newValidator lazily constructs the process-wide protovalidate.Validator on
// first use and memoizes both the validator and any construction error, so the
// (expensive) CEL compilation happens once. Caching the error together with the
// validator means a failed construction is reported consistently on every call
// instead of leaving a nil validator behind a dropped error.
var newValidator = sync.OnceValues(func() (protovalidate.Validator, error) {
	return protovalidate.New()
})

// BuildValidationError converts a ValidationError to a human-readable error message
// combining all violations into a single formatted message.
func BuildValidationError(ve *protovalidate.ValidationError) error {
	bldr := &strings.Builder{}
	bldr.WriteString("validation failed: ")
	for i, violation := range ve.Violations {
		if i > 0 {
			bldr.WriteString("; ")
		}

		if fieldPath := protovalidate.FieldPathString(violation.Proto.GetField()); fieldPath != "" {
			bldr.WriteString(fieldPath)
			bldr.WriteString(": ")
		}

		_, _ = fmt.Fprintf(bldr, "%s [%s]", violation.Proto.GetMessage(), violation.Proto.GetRuleId())
	}
	return errors.New(bldr.String())
}

// ReasonCoder maps a protovalidate rule ID and the violated field name to a
// service-defined, client-facing reason code. The field name is the last segment
// of the violated field path, provided because a "required" violation typically
// derives its code from it. An empty result means "no code": the resulting
// [badrequestv1.FieldViolation.Code] is left unset and downstream consumers fall
// back to the generic gRPC-status reason.
//
// go-atlas ships no built-in mapping — the set of reason codes is part of a
// service's public error contract, so the service supplies its own ReasonCoder
// (see [WithReasonCode]).
type ReasonCoder func(ruleID, fieldName string) string

// validatorConfig holds options for [BuildValidator].
type validatorConfig struct {
	reasonCode ReasonCoder
}

// Option configures [BuildValidator].
type Option func(*validatorConfig)

// WithReasonCode sets the [ReasonCoder] used to derive each field violation's
// canonical reason code. When unset, no code is emitted. A nil coder is ignored.
func WithReasonCode(fn ReasonCoder) Option {
	return func(c *validatorConfig) {
		if fn != nil {
			c.reasonCode = fn
		}
	}
}

// leafFieldName returns the last segment of a violated field path, or "" when the
// path is empty.
func leafFieldName(fp *validate.FieldPath) string {
	elements := fp.GetElements()
	if len(elements) == 0 {
		return ""
	}
	return elements[len(elements)-1].GetFieldName()
}

// BuildValidationFilter creates a filter function that determines which messages
// should be validated. Currently, allows all messages.
func BuildValidationFilter() protovalidate.FilterFunc {
	return func(message protoreflect.Message, descriptor protoreflect.Descriptor) bool {
		return true
	}
}

// BuildValidator creates a protocol buffer message validator function.
// It validates messages using protovalidate and converts errors to gRPC status codes.
func BuildValidator(filter protovalidate.Filter, opts ...Option) func(_ context.Context, msg proto.Message) error {
	var cfg validatorConfig
	for _, opt := range opts {
		opt(&cfg)
	}

	return func(_ context.Context, msg proto.Message) error {
		validator, err := newValidator()
		if err != nil {
			return err
		}

		err = validator.Validate(msg, protovalidate.WithFilter(filter))
		if err == nil {
			return nil
		}

		var ve *protovalidate.ValidationError
		if !errors.As(err, &ve) {
			return err
		}

		fields := make([]*badrequestv1.FieldViolation, 0, len(ve.Violations))
		for _, violation := range ve.Violations {
			field := &badrequestv1.FieldViolation{
				Message:   violation.Proto.Message,
				FieldPath: ptr.WrapNonZero(protovalidate.FieldPathString(violation.Proto.GetField())),
			}

			if cfg.reasonCode != nil {
				if code := cfg.reasonCode(violation.Proto.GetRuleId(), leafFieldName(violation.Proto.GetField())); code != "" {
					field.Code = ptr.Wrap(code)
				}
			}

			if field.FieldPath != nil {
				field.FieldPathComponents = make([]*badrequestv1.FieldPathComponent, 0, len(violation.Proto.GetField().GetElements()))
				for _, element := range violation.Proto.GetField().GetElements() {
					field.FieldPathComponents = append(field.FieldPathComponents, buildFieldPathComponent(element))
				}
			}

			fields = append(fields, field)
		}

		st := status.New(codes.InvalidArgument, "Validation Failed")
		st, _ = st.WithDetails(&badrequestv1.BadRequest{ //nolint:errcheck
			FieldViolations: fields,
		})

		return interceptors.NewError(st, BuildValidationError(ve))
	}
}

// convertFieldType maps protovalidate's *descriptorpb.FieldDescriptorProto_Type
// to *badrequestv1.FieldType. Both enums use identical numeric values
// (1..18 for the concrete proto types), so the conversion is a numeric cast.
func convertFieldType(t *descriptorpb.FieldDescriptorProto_Type) *badrequestv1.FieldType {
	if t == nil {
		return nil
	}
	v := badrequestv1.FieldType(*t)
	return &v
}

// buildFieldPathComponent converts a protovalidate FieldPathElement to a common FieldPathComponent.
func buildFieldPathComponent(element *validate.FieldPathElement) *badrequestv1.FieldPathComponent {
	fieldElement := &badrequestv1.FieldPathComponent{
		Number:       element.FieldNumber,
		Name:         element.FieldName,
		Type:         convertFieldType(element.FieldType),
		MapKeyType:   convertFieldType(element.KeyType),
		MapValueType: convertFieldType(element.ValueType),
	}

	switch s := element.Subscript.(type) {
	case *validate.FieldPathElement_Index:
		fieldElement.IsRepeated = ptr.Wrap(true)
		fieldElement.RepeatedIndex = ptr.Wrap(s.Index)
	case *validate.FieldPathElement_BoolKey:
		fieldElement.MapKey = &badrequestv1.FieldPathComponent_BoolKey{BoolKey: s.BoolKey}
	case *validate.FieldPathElement_IntKey:
		fieldElement.MapKey = &badrequestv1.FieldPathComponent_IntKey{IntKey: s.IntKey}
	case *validate.FieldPathElement_UintKey:
		fieldElement.MapKey = &badrequestv1.FieldPathComponent_UintKey{UintKey: s.UintKey}
	case *validate.FieldPathElement_StringKey:
		fieldElement.MapKey = &badrequestv1.FieldPathComponent_StringKey{StringKey: s.StringKey}
	}

	return fieldElement
}
