// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package jetstream

import "testing"

// White-box: the envelope codec runs once per Submit and per delivery but is
// unexported.

var benchPayload = []byte(`{"id":"order-42","items":[{"sku":"A-1","qty":2},{"sku":"B-7","qty":1}],"total":1999}`)

func BenchmarkEncodeCommand(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		if _, err := encodeCommand("order-42", benchPayload); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDecodeCommand(b *testing.B) {
	body, err := encodeCommand("order-42", benchPayload)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for b.Loop() {
		if _, err := decodeCommand(body); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkMsgID(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		_ = msgID("place-order", "order-42")
	}
}
