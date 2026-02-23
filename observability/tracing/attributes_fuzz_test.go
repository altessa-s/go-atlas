// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package tracing

import "testing"

func FuzzStringAttribute(f *testing.F) {
	f.Add("key", "value")
	f.Add("", "value")
	f.Add("key", "")
	f.Add("http.method", "GET")

	f.Fuzz(func(t *testing.T, key, value string) {
		attr := String(key, value)
		if attr.Key != key {
			t.Errorf("Key = %q, want %q", attr.Key, key)
		}
		if attr.Value != value {
			t.Errorf("Value = %q, want %q", attr.Value, value)
		}
		if key == "" && attr.Valid() {
			t.Error("empty key should be invalid")
		}
		if key != "" && !attr.Valid() {
			t.Error("non-empty key should be valid")
		}
	})
}

func FuzzFilterAttributesByKey(f *testing.F) {
	f.Add("http.")
	f.Add("")
	f.Add("db.system")

	f.Fuzz(func(t *testing.T, prefix string) {
		attrs := []Attribute{
			String("http.method", "GET"),
			String("db.system", "pg"),
			String("rpc.method", "call"),
		}
		count := 0
		for range FilterAttributesByKey(attrs, prefix) {
			count++
		}
		if count > len(attrs) {
			t.Error("filtered more than total")
		}
	})
}
