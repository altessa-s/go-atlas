// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package validationconfig

import (
	ozzo_rules "github.com/altessa-s/ozzo-rules"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// StorageCase pairs a storage type value with the pointer to the sub-config
// that must be set when the type selector equals When.
type StorageCase[T comparable] struct {
	When  T
	Field any
}

// ValidateStorage validates a storage selector: *typePtr must be one of
// allowed, and the Field of the case matching it must be non-nil.
func ValidateStorage[T comparable](structPtr any, typePtr *T, allowed []T, cases []StorageCase[T]) error {
	allowedAny := make([]any, 0, len(allowed))
	for _, v := range allowed {
		allowedAny = append(allowedAny, v)
	}

	fields := make([]*validation.FieldRules, 0, 1+len(cases))
	fields = append(fields,
		validation.Field(typePtr, validation.Required, ozzo_rules.OneOf(allowedAny...)),
	)

	for _, c := range cases {
		fields = append(fields,
			validation.Field(c.Field, validation.When(*typePtr == c.When, validation.NilOrNotEmpty)),
		)
	}

	return ValidateStruct(structPtr, fields...)
}
