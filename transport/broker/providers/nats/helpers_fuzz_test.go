// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package natsprovider

import "testing"

func FuzzIsValidSubject(f *testing.F) {
	f.Add("my-topic")
	f.Add("")
	f.Add("topic.dots")
	f.Add("a-b-c-123")

	f.Fuzz(func(t *testing.T, subject string) {
		// Should not panic
		IsValidSubject(subject)
	})
}
