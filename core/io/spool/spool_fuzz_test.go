// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package spool_test

import (
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/core/io/spool"
)

// FuzzSpoolPreservesItsInput pins the one thing a rewindable reader owes its
// caller: what comes out is what went in, whichever backing store it chose.
//
// A Spool keeps small payloads in memory and spills larger ones to a temp file,
// so a single body crosses two entirely different code paths depending on its
// size. The threshold is where a boundary bug lives — an off-by-one in the head
// buffer loses or duplicates the bytes straddling it, and the corruption
// reaches whatever parsed the body as a malformed request rather than as an I/O
// error.
func FuzzSpoolPreservesItsInput(f *testing.F) {
	f.Add([]byte("small"), int64(1024))
	f.Add([]byte(""), int64(8))
	f.Add(bytes.Repeat([]byte("x"), 100), int64(1))   // Spills immediately.
	f.Add(bytes.Repeat([]byte("x"), 100), int64(100)) // Exactly at the threshold.
	f.Add(bytes.Repeat([]byte("x"), 100), int64(99))  // One byte over.
	f.Add([]byte{0x00, 0xff, 0x0a}, int64(2))

	f.Fuzz(func(t *testing.T, payload []byte, threshold int64) {
		if threshold < 0 || threshold > 1<<20 {
			t.Skip("thresholds outside the plausible range say nothing about the code")
		}

		s, err := spool.New(bytes.NewReader(payload), spool.WithMemThreshold(threshold))
		require.NoError(t, err)
		t.Cleanup(func() { _ = s.Close() })

		require.Equal(t, int64(len(payload)), s.Size(),
			"the reported size disagrees with the payload at threshold %d", threshold)

		got, err := io.ReadAll(s)
		require.NoError(t, err)
		// bytes.Equal rather than require.Equal: an empty payload and an empty
		// read differ only in nil-ness, which is not a property of the spool.
		require.True(t, bytes.Equal(payload, got),
			"the payload changed at threshold %d: %q became %q", threshold, payload, got)
	})
}

// FuzzSpoolRewindsToTheSameBytes pins what "rewindable" means: every pass over
// the spool yields the same content.
//
// The whole point of spooling a body is that something can read it twice — a
// signature check and then a handler, say. A second pass that differs from the
// first is a verified body that is not the one the handler sees, which is the
// hmacsign hazard in another guise.
func FuzzSpoolRewindsToTheSameBytes(f *testing.F) {
	f.Add([]byte("payload"), int64(4))
	f.Add([]byte(""), int64(0))
	f.Add(bytes.Repeat([]byte("ab"), 512), int64(64))

	f.Fuzz(func(t *testing.T, payload []byte, threshold int64) {
		if threshold < 0 || threshold > 1<<20 {
			t.Skip("thresholds outside the plausible range say nothing about the code")
		}

		s, err := spool.New(bytes.NewReader(payload), spool.WithMemThreshold(threshold))
		require.NoError(t, err)
		t.Cleanup(func() { _ = s.Close() })

		first, err := io.ReadAll(s)
		require.NoError(t, err)

		_, err = s.Seek(0, io.SeekStart)
		require.NoError(t, err)

		second, err := io.ReadAll(s)
		require.NoError(t, err)

		require.True(t, bytes.Equal(first, second), "a rewound spool yielded different bytes")
	})
}

// FuzzSpoolTeeSeesEveryByte pins that the tee is a faithful copy of the source,
// not of whatever the spool happened to keep.
//
// A tee is how a caller hashes or counts a body while spooling it. If it can
// miss the bytes that spilled to disk — or see the head twice — a checksum
// computed through it authenticates something other than the payload.
func FuzzSpoolTeeSeesEveryByte(f *testing.F) {
	f.Add([]byte("payload"), int64(4))
	f.Add([]byte(""), int64(0))
	f.Add(bytes.Repeat([]byte("z"), 300), int64(128))

	f.Fuzz(func(t *testing.T, payload []byte, threshold int64) {
		if threshold < 0 || threshold > 1<<20 {
			t.Skip("thresholds outside the plausible range say nothing about the code")
		}

		var teed bytes.Buffer
		s, err := spool.New(bytes.NewReader(payload),
			spool.WithMemThreshold(threshold), spool.WithTee(&teed))
		require.NoError(t, err)
		t.Cleanup(func() { _ = s.Close() })

		require.True(t, bytes.Equal(payload, teed.Bytes()),
			"the tee saw something other than the source at threshold %d: %q vs %q",
			threshold, payload, teed.Bytes())
	})
}

// FuzzSpoolRefusesOversizedInput pins the cap: a payload past MaxBytes is
// refused, and one within it is not.
//
// The cap is what keeps an unbounded request body from becoming an unbounded
// temp file. A limit that admits one byte too many is a limit an attacker sizes
// their payload to.
func FuzzSpoolRefusesOversizedInput(f *testing.F) {
	f.Add([]byte("0123456789"), int64(5), int64(4))
	f.Add([]byte("0123456789"), int64(10), int64(4))
	f.Add([]byte(""), int64(0), int64(0))

	f.Fuzz(func(t *testing.T, payload []byte, maxBytes, threshold int64) {
		if maxBytes <= 0 || maxBytes > 1<<20 || threshold < 0 || threshold > 1<<20 {
			t.Skip("limits outside the plausible range say nothing about the code")
		}

		s, err := spool.New(bytes.NewReader(payload),
			spool.WithMaxBytes(maxBytes), spool.WithMemThreshold(threshold))
		if err != nil {
			require.Greater(t, int64(len(payload)), maxBytes,
				"a payload within the %d-byte cap was refused", maxBytes)
			return
		}
		t.Cleanup(func() { _ = s.Close() })

		require.LessOrEqual(t, int64(len(payload)), maxBytes,
			"a payload past the %d-byte cap was accepted", maxBytes)
	})
}
