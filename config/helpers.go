// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config

import (
	"fmt"
	"strings"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// ValidationErrorsToFlatMap converts nested validation.Errors into a flat map structure.
// It recursively flattens nested validation errors using dot notation for keys.
// This is useful for displaying validation errors in a user-friendly format.
//
// Example:
//
//	errs := validation.Errors{
//	  "server": validation.Errors{
//	    "port": fmt.Errorf("invalid port"),
//	  },
//	}
//	flat := ValidationErrorsToFlatMap(errs)
//	// Result: {"server.port": "invalid port"}
func ValidationErrorsToFlatMap(in validation.Errors) validation.Errors {
	out := make(validation.Errors)
	for key, value := range in {
		if valueAsMap, ok := value.(validation.Errors); ok {
			sub := ValidationErrorsToFlatMap(valueAsMap)
			for subKey, subValue := range sub {
				out[key+"."+subKey] = subValue
			}
			continue
		}
		out[key] = value
	}
	return out
}

// PrettyError formats validation errors into a human-readable string.
// If the error is a validation.Errors type, it flattens the structure and
// formats each error with field names. For other error types, it returns
// the standard error message.
//
// Example output:
//
//	Configuration in not valid:
//	  - server.port: invalid port number
//	  - database.host: cannot be blank
func PrettyError(err error) string {
	if err == nil {
		return ""
	}
	if verrs, ok := coreerrs.AsType[validation.Errors](err); ok {
		verrs = ValidationErrorsToFlatMap(verrs)
		var strs = make([]string, 0, len(verrs))
		for f, e := range verrs {
			strs = append(strs, fmt.Sprintf("  - %s: %v", f, e))
		}
		return "Configuration in not valid:\n" +
			strings.Join(strs, "\n")
	}
	return err.Error()
}
