// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package json

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

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
		assert.Equal(t, nil, c.Decode(encoded, &decoded))
		assert.Equal(t, value, decoded[key])
	})
}
