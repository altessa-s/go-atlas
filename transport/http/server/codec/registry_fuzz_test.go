// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package codec

import "testing"

func FuzzRegistry_Negotiate(f *testing.F) {
	f.Add("application/json")
	f.Add("application/xml, application/json;q=0.9")
	f.Add("*/*")
	f.Add("")
	f.Add("text/html")

	r := NewRegistry()
	r.RegisterEncoder("application/json", &mockCodec{mime: "application/json"})

	f.Fuzz(func(t *testing.T, accept string) {
		_, _, _ = r.Negotiate(accept) // Should not panic
	})
}
