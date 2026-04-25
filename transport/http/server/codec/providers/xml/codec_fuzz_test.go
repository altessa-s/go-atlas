// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package xml

import (
	"encoding/xml"
	"testing"
)

func FuzzDecode(f *testing.F) {
	f.Add([]byte(`<Item><Name>test</Name></Item>`))
	f.Add([]byte(`invalid`))
	f.Add([]byte(`<empty/>`))
	f.Add([]byte(``))

	c := New()

	f.Fuzz(func(t *testing.T, data []byte) {
		var out struct {
			XMLName xml.Name `xml:"Item"`
			Name    string   `xml:"Name"`
		}
		_ = c.Decode(data, &out) // Should not panic
	})
}
