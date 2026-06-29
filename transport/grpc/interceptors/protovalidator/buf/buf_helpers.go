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

	"github.com/altessa-s/go-atlas/core/types/ptr"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/protovalidator/reasoncode"

	"github.com/altessa-s/proto-gen-go/badrequest/v1"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
)

var (
	// validator is the singleton protovalidate.Validator instance.
	validator protovalidate.Validator
	// validatorOnce ensures validator is initialized only once.
	validatorOnce sync.Once
)

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

// defaultResolver maps the standard protovalidate rule IDs to canonical reason
// codes. It carries no catalog; services supply their own via [WithResolver].
var defaultResolver = reasoncode.NewResolver(nil)

// BuildErrorCode generates a canonical reason code from a rule ID and field path
// using the default (standard-rules-only) resolver. For "required" violations it
// returns "{FIELD_NAME}_REQUIRED" from the last field-path component; every other
// rule is mapped to a canonical code (see package [reasoncode]). It has no
// message, so it cannot resolve numeric range rules; use [BuildValidator] for
// full resolution against a service catalog.
func BuildErrorCode(ruleID string, fieldPath *validate.FieldPath) string {
	return defaultResolver.ResolveViolation(newViolationView(ruleID, fieldPath, nil))
}

// validatorConfig holds options for [BuildValidator].
type validatorConfig struct {
	resolver *reasoncode.Resolver
}

// Option configures [BuildValidator].
type Option func(*validatorConfig)

// WithResolver sets the resolver used to translate validation rule IDs into
// canonical reason codes, letting a service supply its own catalog (see
// [reasoncode.NewResolver]). When unset, only the standard rules are resolved.
// A nil resolver is ignored.
func WithResolver(r *reasoncode.Resolver) Option {
	return func(c *validatorConfig) {
		if r != nil {
			c.resolver = r
		}
	}
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
	cfg := validatorConfig{resolver: defaultResolver}
	for _, opt := range opts {
		opt(&cfg)
	}

	return func(_ context.Context, msg proto.Message) error {
		var err error
		validatorOnce.Do(func() {
			validator, err = protovalidate.New()
		})
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
				Code:      ptr.Wrap(cfg.resolver.ResolveViolation(newViolationView(violation.Proto.GetRuleId(), violation.Proto.GetField(), msg))),
				FieldPath: ptr.WrapNonZero(protovalidate.FieldPathString(violation.Proto.GetField())),
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
