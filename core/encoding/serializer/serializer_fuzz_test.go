// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package serializer

import (
	"testing"
)

func FuzzJSONRoundTrip(f *testing.F) {
	f.Add(`{"key":"value"}`)
	f.Add(`{"num":42,"arr":[1,2,3]}`)
	f.Add(`{}`)

	s := &JSON{}
	f.Fuzz(func(t *testing.T, input string) {
		// Try to deserialize
		var data any
		if err := s.Deserialize([]byte(input), &data); err != nil {
			return // invalid JSON, skip
		}

		// Re-serialize
		out, err := s.Serialize(data)
		if err != nil {
			t.Fatalf("Serialize failed after successful Deserialize: %v", err)
		}

		// Deserialize again and compare
		var data2 any
		if err := s.Deserialize(out, &data2); err != nil {
			t.Fatalf("second Deserialize failed: %v", err)
		}
	})
}
