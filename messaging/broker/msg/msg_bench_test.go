// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package msg

import "testing"

func BenchmarkNewMessage(b *testing.B) {
	data := []byte(`{"type":"created"}`)
	for b.Loop() {
		NewMessage("events", data)
	}
}

func BenchmarkNewMessageWithMeta(b *testing.B) {
	data := []byte(`{"type":"created"}`)
	meta := Meta{{Key: "trace_id", Value: "abc123"}}
	for b.Loop() {
		NewMessageWithMeta("events", data, meta)
	}
}

func BenchmarkMeta_Map(b *testing.B) {
	meta := Meta{
		{Key: "k1", Value: "v1"},
		{Key: "k2", Value: "v2"},
		{Key: "k3", Value: "v3"},
	}
	for b.Loop() {
		meta.Map()
	}
}

func BenchmarkMeta_Value(b *testing.B) {
	meta := Meta{
		{Key: "k1", Value: "v1"},
		{Key: "k2", Value: "v2"},
		{Key: "k3", Value: "v3"},
	}
	for b.Loop() {
		meta.Value("k2")
	}
}

func BenchmarkMetaFromMap(b *testing.B) {
	m := map[string]string{"k1": "v1", "k2": "v2", "k3": "v3"}
	for b.Loop() {
		MetaFromMap(m)
	}
}
