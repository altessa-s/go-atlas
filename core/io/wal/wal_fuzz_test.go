// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package wal_test

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/core/io/wal"
)

// firstSegment is the name Open assigns to segment 1, which is what recovery
// looks for. A file under any other name is ignored rather than recovered.
const firstSegment = "00000000000000000001.wal"

// maxFuzzPayload keeps a generated payload inside what Append accepts, so a
// rejected write cannot be mistaken for a recovery failure.
const maxFuzzPayload = 1 << 16

// encodeRecord renders one on-disk record: [uint32 length][uint32 crc32][payload].
func encodeRecord(payload []byte) []byte {
	out := make([]byte, 8, 8+len(payload))
	binary.LittleEndian.PutUint32(out[0:4], uint32(len(payload)))
	binary.LittleEndian.PutUint32(out[4:8], crc32.ChecksumIEEE(payload))
	return append(out, payload...)
}

// onlySegment returns the path of the single segment file in dir.
func onlySegment(tb testing.TB, dir string) string {
	tb.Helper()

	paths, err := filepath.Glob(filepath.Join(dir, "*.wal"))
	require.NoError(tb, err)
	require.Len(tb, paths, 1, "the fixture writes one segment; got %v", paths)

	return paths[0]
}

// FuzzRecoverArbitrarySegment points recovery at a segment file of arbitrary
// bytes — the shape a segment has after a crash mid-write, a partial fsync, or
// bit rot on the volume.
//
// The record header carries an attacker-visible 32-bit length that recovery
// uses to size an allocation, so "does not panic" is the floor, not the point.
// The assertions pin what recovery may output: only data that was literally in
// the file, never a zero-length record (which the format cannot represent and
// which would truncate every record written after it on the next Open).
func FuzzRecoverArbitrarySegment(f *testing.F) {
	f.Add([]byte{})
	f.Add([]byte{0x01, 0x00, 0x00, 0x00})                    // Header cut short.
	f.Add(encodeRecord([]byte("hello")))                     // One good record.
	f.Add(append(encodeRecord([]byte("hello")), 0xff, 0xff)) // Good record, torn tail.
	f.Add([]byte{0xff, 0xff, 0xff, 0xff, 0, 0, 0, 0})        // length = MaxUint32.
	f.Add([]byte{0, 0, 0, 0, 0, 0, 0, 0})                    // length = 0.
	f.Add(func() []byte {                                    // Good record whose CRC was flipped.
		rec := encodeRecord([]byte("hello"))
		rec[4] ^= 0xff
		return rec
	}())

	f.Fuzz(func(t *testing.T, raw []byte) {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, firstSegment), raw, 0o600))

		w, recovered, err := wal.Open(dir)
		if err != nil {
			return // Refusing a directory is a legitimate outcome; panicking is not.
		}
		defer func() { _ = w.Close() }()

		for i, rec := range recovered {
			require.NotEmpty(t, rec.Payload,
				"record %d: the format cannot represent a zero-length payload, so recovery must never produce one", i)
			require.True(t, bytes.Contains(raw, rec.Payload),
				"record %d: recovery returned bytes that were not in the segment", i)
		}
	})
}

// FuzzRecoverReturnsAnUnbrokenPrefix is the property a write-ahead log lives or
// dies by: after a crash, what comes back must be a prefix of what went in.
//
// Losing the tail is expected — the crash landed somewhere in it. Losing a
// record from the middle, or reordering two, would mean a consumer replays a
// state the producer never wrote. Truncating the segment at an arbitrary offset
// models the crash; the assertion is that recovery degrades by dropping a
// suffix and in no other way.
func FuzzRecoverReturnsAnUnbrokenPrefix(f *testing.F) {
	f.Add([]byte("alpha"), []byte("beta"), uint16(0))
	f.Add([]byte("alpha"), []byte("beta"), uint16(1))  // Torn last record.
	f.Add([]byte("alpha"), []byte("beta"), uint16(6))  // Last record's header gone.
	f.Add([]byte("a"), []byte("b"), uint16(64))        // Truncated past the start.
	f.Add([]byte{0x00, 0xff}, []byte{0x7f}, uint16(3)) // Binary payloads.

	f.Fuzz(func(t *testing.T, first, second []byte, cut uint16) {
		if len(first) == 0 || len(second) == 0 {
			t.Skip("Append rejects empty payloads by contract")
		}
		if len(first) > maxFuzzPayload || len(second) > maxFuzzPayload {
			t.Skip("oversized payload; Append's size limit is not what this target is about")
		}

		dir := t.TempDir()

		w, _, err := wal.Open(dir)
		require.NoError(t, err)
		_, err = w.Append(first)
		require.NoError(t, err)
		_, err = w.Append(second)
		require.NoError(t, err)
		require.NoError(t, w.Close())

		// Cut the tail off, the way a crash leaves a partially written segment.
		path := onlySegment(t, dir)
		info, err := os.Stat(path)
		require.NoError(t, err)
		require.NoError(t, os.Truncate(path, max(info.Size()-int64(cut), 0)))

		reopened, recovered, err := wal.Open(dir)
		require.NoError(t, err)
		defer func() { _ = reopened.Close() }()

		want := [][]byte{first, second}
		require.LessOrEqual(t, len(recovered), len(want),
			"recovery produced more records than were ever appended")
		for i, rec := range recovered {
			require.Equal(t, want[i], rec.Payload,
				"record %d differs: recovery must return an unbroken prefix, never a gap or a reorder", i)
		}
	})
}
