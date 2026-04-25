// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package client

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func FuzzFieldError_Error(f *testing.F) {
	f.Add("email", "invalid")
	f.Add("", "")
	f.Add("user.name", "too long")

	f.Fuzz(func(t *testing.T, field, message string) {
		fe := &FieldError{Field: field, Message: message}
		result := fe.Error()
		assert.NotEqual(t, "", result)
	})
}

func FuzzError_ReasonIsOneof(f *testing.F) {
	f.Add("REASON_A", "REASON_A")
	f.Add("REASON_B", "OTHER")

	f.Fuzz(func(t *testing.T, reason, check string) {
		e := &Error{Reason: reason}
		// Should not panic
		e.ReasonIsOneof(check)
	})
}
