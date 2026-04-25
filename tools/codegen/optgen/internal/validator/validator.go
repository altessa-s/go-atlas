// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package validator

import (
	"fmt"
	"unicode"
)

// ValidateGoIdentifier checks if a string is a valid Go identifier.
// A valid identifier must:
//   - Start with a letter or underscore
//   - Contain only letters, digits, and underscores
func ValidateGoIdentifier(name string) error {
	if name == "" {
		return fmt.Errorf("identifier cannot be empty")
	}
	if !unicode.IsLetter(rune(name[0])) && name[0] != '_' {
		return fmt.Errorf("identifier must start with a letter or underscore: %s", name)
	}
	for _, r := range name {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' {
			return fmt.Errorf("identifier contains invalid character: %s", name)
		}
	}
	return nil
}
