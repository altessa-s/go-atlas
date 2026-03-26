package buf

import (
	"errors"
	"fmt"
	"strings"

	"buf.build/gen/go/bufbuild/protovalidate/protocolbuffers/go/buf/validate"
	"buf.build/go/protovalidate"

	corestrings "github.com/altessa-s/go-atlas/core/text/strings"
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
