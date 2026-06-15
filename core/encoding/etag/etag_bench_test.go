// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package etag_test

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/altessa-s/go-atlas/core/encoding/etag"
)

func BenchmarkHash(b *testing.B) {
	g := etag.NewGenerator()
	small := []byte("hello world")
	large := bytes.Repeat([]byte("a"), 64<<10)

	b.Run("small", func(b *testing.B) {
		for b.Loop() {
			_ = g.Hash(small)
		}
	})
	b.Run("large", func(b *testing.B) {
		for b.Loop() {
			_ = g.Hash(large)
		}
	})
}

func BenchmarkHashString(b *testing.B) {
	g := etag.NewGenerator()
	s := strings.Repeat("a", 1024)
	for b.Loop() {
		_ = g.HashString(s)
	}
}

func BenchmarkHashReader(b *testing.B) {
	g := etag.NewGenerator()
	data := bytes.Repeat([]byte("a"), 64<<10)
	r := bytes.NewReader(data)
	for b.Loop() {
		r.Reset(data)
		_, _ = g.HashReader(r)
	}
}

func BenchmarkFromModTime(b *testing.B) {
	mod := time.Unix(0, 0x1234abcd)
	for b.Loop() {
		_ = etag.FromModTime(4096, mod)
	}
}

func BenchmarkParse(b *testing.B) {
	for b.Loop() {
		_, _ = etag.Parse(`W/"686d6f3e2c"`)
	}
}
