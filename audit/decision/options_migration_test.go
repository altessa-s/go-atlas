// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package decision

import "testing"

func TestWithAttributeSanitizerBaseline(t *testing.T) {
	got := defaultOptions()
	fn := func(_, value string) string { return " " + value + " " }
	WithAttributeSanitizer(fn)(got)
	if got.attributeSanitizer("key", "value") != " value " {
		t.Fatal("sanitizer not applied")
	}
	WithAttributeSanitizer(nil)(got)
	if got.attributeSanitizer != nil {
		t.Fatal("nil sanitizer did not clear the configured value")
	}
}
