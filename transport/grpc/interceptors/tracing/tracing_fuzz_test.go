// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package tracing

import "testing"

func FuzzParseMethod(f *testing.F) {
	f.Add("/mypackage.MyService/MyMethod")
	f.Add("")
	f.Add("/")
	f.Add("noSlash")
	f.Add("/ServiceOnly")
	f.Fuzz(func(t *testing.T, method string) {
		svc, m := parseMethod(method)
		_ = svc
		_ = m
	})
}
