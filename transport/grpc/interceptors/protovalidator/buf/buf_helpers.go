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

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	protovalidatev1 "github.com/altessa-s/atlas-proto-gen-go/protovalidate/v1"
	corestrings "github.com/altessa-s/go-atlas/core/text/strings"
)

var (
	// validator is the singleton protovalidate.Validator instance.
	validator protovalidate.Validator
	// validatorOnce ensures validator is initialized only once.
	validatorOnce sync.Once
)

const (
	// ruleIDRequired is the rule ID for required field violations.
	ruleIDRequired = "required"

	// requiredSuffix is the suffix appended to field names for required violations.
	requiredSuffix = "_REQUIRED"

	// requiredFallback is the fallback error code when field name cannot be extracted.
	requiredFallback = "REQUIRED"
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

// BuildErrorCode generates an error code based on the rule ID and field path.
// For "required" rule violations, it returns "{FIELD_NAME}_REQUIRED" where FIELD_NAME
// is the last component of the field path in uppercase.
// For other violations, it returns the uppercase rule ID.
func BuildErrorCode(ruleID string, fieldPath *validate.FieldPath) string {
	if ruleID == "" {
		return ""
	}

	// For required field violations, format as FIELD_NAME_REQUIRED
	if strings.ToLower(ruleID) == ruleIDRequired {
		if fieldPath != nil {
			elements := fieldPath.GetElements()
			if len(elements) > 0 {
				// Get the last element's field name (for nested fields)
				lastName := elements[len(elements)-1].GetFieldName()
				if lastName != "" {
					return corestrings.ToScreamingSnakeCase(lastName) + requiredSuffix
				}
			}
		}
		// Fallback if we can't extract field name
		return requiredFallback
	}

	// For other violations, return uppercase rule ID
	return strings.ToUpper(ruleID)
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
func BuildValidator(filter protovalidate.Filter) func(_ context.Context, msg proto.Message) error {
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

		fields := make([]*protovalidatev1.FieldViolation, 0, len(ve.Violations))
		for _, violation := range ve.Violations {
			field := &protovalidatev1.FieldViolation{
				Message:   violation.Proto.Message,
				Code:      ptr.Wrap(BuildErrorCode(violation.Proto.GetRuleId(), violation.Proto.GetField())),
				FieldPath: ptr.WrapNonZero(protovalidate.FieldPathString(violation.Proto.GetField())),
			}

			if field.FieldPath != nil {
				field.FieldPathComponents = make([]*protovalidatev1.FieldPathComponent, 0, len(violation.Proto.GetField().GetElements()))
				for _, element := range violation.Proto.GetField().GetElements() {
					field.FieldPathComponents = append(field.FieldPathComponents, buildFieldPathComponent(element))
				}
			}

			fields = append(fields, field)
		}

		st := status.New(codes.InvalidArgument, "Validation Failed")
		st, _ = st.WithDetails(&protovalidatev1.BadRequest{ //nolint:errcheck
			FieldViolations: fields,
		})

		return interceptors.NewError(st, BuildValidationError(ve))
	}
}

// buildFieldPathComponent converts a protovalidate FieldPathElement to a common FieldPathComponent.
func buildFieldPathComponent(element *validate.FieldPathElement) *protovalidatev1.FieldPathComponent {
	fieldElement := &protovalidatev1.FieldPathComponent{
		Number:       element.FieldNumber,
		Name:         element.FieldName,
		Type:         element.FieldType,
		MapKeyType:   element.KeyType,
		MapValueType: element.ValueType,
	}

	switch s := element.Subscript.(type) {
	case *validate.FieldPathElement_Index:
		fieldElement.IsRepeated = ptr.Wrap(true)
		fieldElement.RepeatedIndex = ptr.Wrap(s.Index)
	case *validate.FieldPathElement_BoolKey:
		fieldElement.MapKey = &protovalidatev1.FieldPathComponent_BoolKey{BoolKey: s.BoolKey}
	case *validate.FieldPathElement_IntKey:
		fieldElement.MapKey = &protovalidatev1.FieldPathComponent_IntKey{IntKey: s.IntKey}
	case *validate.FieldPathElement_UintKey:
		fieldElement.MapKey = &protovalidatev1.FieldPathComponent_UintKey{UintKey: s.UintKey}
	case *validate.FieldPathElement_StringKey:
		fieldElement.MapKey = &protovalidatev1.FieldPathComponent_StringKey{StringKey: s.StringKey}
	}

	return fieldElement
}
