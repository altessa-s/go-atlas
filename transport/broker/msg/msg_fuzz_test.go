// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package msg

import "testing"

func FuzzNewMessage(f *testing.F) {
	f.Add("events", []byte("hello"))
	f.Add("", []byte(""))
	f.Add("topic.with.dots", []byte(`{"key":"value"}`))

	f.Fuzz(func(t *testing.T, topic string, data []byte) {
		m := NewMessage(topic, data)
		if m == nil {
			t.Fatal("NewMessage returned nil")
		}
		if m.Topic != topic {
			t.Fatalf("Topic = %q, want %q", m.Topic, topic)
		}
		if len(data) != len(m.Data) {
			t.Fatalf("Data length = %d, want %d", len(m.Data), len(data))
		}
	})
}

func FuzzMeta_Value(f *testing.F) {
	f.Add("key1")
	f.Add("")
	f.Add("missing")

	meta := Meta{
		{Key: "key1", Value: "val1"},
		{Key: "key2", Value: "val2"},
	}

	f.Fuzz(func(t *testing.T, key string) {
		// Should not panic
		meta.Value(key)
	})
}
