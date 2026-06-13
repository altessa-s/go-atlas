// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package io_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	coreio "github.com/altessa-s/go-atlas/core/io"
)

func BenchmarkGetPutBuffer(b *testing.B) {
	for b.Loop() {
		buf := coreio.GetBuffer()
		buf.WriteString("payload")
		coreio.PutBuffer(buf)
	}
}

// BenchmarkLimitedReadCloser_Read constructs the reader inside the timed loop:
// a LimitedReadCloser wraps a consumed underlying reader and cannot be rewound,
// so per-iteration construction is unavoidable and included in the measurement.
func BenchmarkLimitedReadCloser_Read(b *testing.B) {
	payload := strings.Repeat("x", 1024)
	dst := make([]byte, 256)

	for b.Loop() {
		lrc := coreio.NewLimitedReadCloser(io.NopCloser(strings.NewReader(payload)), int64(len(payload)))
		for {
			_, err := lrc.Read(dst)
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				b.Fatal(err)
			}
		}
	}
}

func BenchmarkRangeReadSeeker_Read(b *testing.B) {
	data := []byte(strings.Repeat("x", 1024))
	open := func(_ context.Context, offset, length int64) (io.ReadCloser, error) {
		end := min(offset+length, int64(len(data)))
		return io.NopCloser(bytes.NewReader(data[offset:end])), nil
	}
	dst := make([]byte, 256)

	for b.Loop() {
		rs := coreio.NewRangeReadSeeker(b.Context(), int64(len(data)), open)
		for {
			_, err := rs.Read(dst)
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				b.Fatal(err)
			}
		}
		rs.Close()
	}
}
