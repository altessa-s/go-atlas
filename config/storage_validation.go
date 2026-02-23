// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config

import (
	ozzo_rules "github.com/altessa-s/ozzo-rules"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

type storageCase[T comparable] struct {
	when  T
	field any
}

func validateStorageConfig[T comparable](structPtr any, typePtr *T, allowed []T, cases []storageCase[T]) error {
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
			validation.Field(c.field, validation.When(*typePtr == c.when, validation.NilOrNotEmpty)),
		)
	}

	return ValidateStruct(structPtr, fields...)
}
