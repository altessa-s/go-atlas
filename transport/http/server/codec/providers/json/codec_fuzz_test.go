// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package json

import "testing"

func FuzzDecode(f *testing.F) {
	f.Add([]byte(`{"key":"value"}`))
	f.Add([]byte(`invalid`))
	f.Add([]byte(`[]`))
	f.Add([]byte(`null`))
	f.Add([]byte(``))

	c := New()

	f.Fuzz(func(t *testing.T, data []byte) {
		var out any
		_ = c.Decode(data, &out) // Should not panic
	})
}

func FuzzEncodeDecode(f *testing.F) {
	f.Add("key", "value")
	f.Add("", "")
	f.Add("special<>&", "chars\"quote")

	c := New()

	f.Fuzz(func(t *testing.T, key, value string) {
		input := map[string]string{key: value}
		encoded, err := c.Encode(input)
		if err != nil {
			return
		}
		var decoded map[string]string
		if err := c.Decode(encoded, &decoded); err != nil {
			t.Fatalf("roundtrip failed: encode ok but decode error = %v", err)
		}
		if decoded[key] != value {
			t.Fatalf("roundtrip mismatch: got %q, want %q", decoded[key], value)
		}
	})
}
