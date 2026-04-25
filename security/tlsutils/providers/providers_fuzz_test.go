// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package tlsproviders

import "testing"

func FuzzProviderType_IsValid(f *testing.F) {
	f.Add("vault")
	f.Add("file")
	f.Add("letsencrypt")
	f.Add("s3")
	f.Add("unknown")
	f.Add("")

	f.Fuzz(func(t *testing.T, s string) {
		pt := ProviderType(s)
		_ = pt.IsValid()
		_ = pt.String()
	})
}
