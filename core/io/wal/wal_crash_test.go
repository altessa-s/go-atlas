// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package wal_test

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/core/io/wal"
)

// crashChildDirEnv marks the re-executed test binary as the crash child and
// names the WAL directory it appends to.
const crashChildDirEnv = "ATLAS_WAL_CRASH_CHILD_DIR"

// crashSegmentBytes is small so every child run rolls several segments.
const crashSegmentBytes = 8 << 10

// recordPayload encodes id followed by a filler derived from it, so a reader
// can tell a corrupted or misattributed payload from a valid one.
func recordPayload(id uint64) []byte {
	p := make([]byte, 8+int(id%97)+1)
	binary.LittleEndian.PutUint64(p, id)
	for i := 8; i < len(p); i++ {
		p[i] = byte(id) ^ byte(i)
	}
	return p
}

// recordID decodes and validates a payload built by recordPayload.
func recordID(p []byte) (uint64, bool) {
	if len(p) < 9 {
		return 0, false
	}
	id := binary.LittleEndian.Uint64(p)
	return id, bytes.Equal(p, recordPayload(id))
}

// TestWALCrashRecovery kills a child process with SIGKILL while it appends,
// then reopens the WAL. Recovery must return a gap-free prefix 1..N of valid
// records that includes every record the child reported as synced. Every
// other iteration also appends a torn record to the newest segment, which
// recovery must drop. The child resumes on the recovered log, so later
// iterations recover across segments written by several processes.
func TestWALCrashRecovery(t *testing.T) {
	if dir := os.Getenv(crashChildDirEnv); dir != "" {
		runCrashChild(t, dir)
		return
	}
	t.Parallel()

	const iterations = 6
	dir := t.TempDir()
	require.NoError(t, os.Chmod(dir, 0o700))
	var lastSynced uint64
	for iter := range iterations {
		synced := runAndKillChild(t, dir)
		require.GreaterOrEqual(t, synced, lastSynced, "iteration %d: child reported fewer synced records than recovered before", iter)
		if iter%2 == 1 {
			appendTornRecord(t, dir)
		}

		w, recovered, err := wal.Open(dir, wal.WithMaxSegmentBytes(crashSegmentBytes))
		require.NoError(t, err, "iteration %d", iter)
		for i, r := range recovered {
			id, ok := recordID(r.Payload)
			require.True(t, ok, "iteration %d: corrupt record recovered at %+v", iter, r.Offset)
			require.Equal(t, uint64(i+1), id, "iteration %d: recovered records are not a gap-free prefix", iter)
		}
		n := uint64(len(recovered))
		require.GreaterOrEqual(t, n, synced, "iteration %d: records synced before the kill were lost", iter)
		require.NoError(t, w.Close())
		t.Logf("iteration %d: synced=%d recovered=%d", iter, synced, n)
		lastSynced = n
	}
}

// runAndKillChild re-executes the test binary as the crash child, kills it
// with SIGKILL at a random point once it has made progress, and returns the
// highest record ID the child reported as appended and synced.
func runAndKillChild(t *testing.T, dir string) uint64 {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestWALCrashRecovery$", "-test.count=1") //nolint:gosec // re-executes this test binary
	cmd.Env = append(os.Environ(), crashChildDirEnv+"="+dir)
	cmd.Stderr = os.Stderr
	stdout, err := cmd.StdoutPipe()
	require.NoError(t, err)
	require.NoError(t, cmd.Start())

	lines := make(chan uint64, 1024)
	go func() {
		defer close(lines)
		sc := bufio.NewScanner(stdout)
		for sc.Scan() {
			if v, ok := strings.CutPrefix(sc.Text(), "synced "); ok {
				if id, err := strconv.ParseUint(v, 10, 64); err == nil {
					lines <- id
				}
			}
		}
	}()

	var synced uint64
	select {
	case id, ok := <-lines:
		require.True(t, ok, "crash child exited before syncing anything")
		synced = id
	case <-time.After(10 * time.Second):
		require.FailNow(t, "crash child made no progress within 10s")
	}
	kill := time.After(time.Duration(rand.IntN(200)) * time.Millisecond) //nolint:gosec // test jitter
wait:
	for {
		select {
		case id := <-lines:
			synced = max(synced, id)
		case <-kill:
			break wait
		}
	}
	require.NoError(t, cmd.Process.Kill())
	// Lines already in the pipe were printed after a completed Sync.
	for id := range lines {
		synced = max(synced, id)
	}
	_ = cmd.Wait() //nolint:errcheck // killed on purpose
	return synced
}

// runCrashChild appends records forever, continuing after the highest
// recovered ID, and prints "synced <id>" after every Sync.
func runCrashChild(t *testing.T, dir string) {
	w, recovered, err := wal.Open(dir, wal.WithMaxSegmentBytes(crashSegmentBytes), wal.WithFsyncInterval(time.Millisecond))
	require.NoError(t, err)
	next := uint64(len(recovered)) + 1
	deadline := time.Now().Add(time.Minute) // never outlive a lost parent
	for ; time.Now().Before(deadline); next++ {
		_, err := w.Append(recordPayload(next))
		require.NoError(t, err)
		if next%16 == 0 {
			require.NoError(t, w.Sync())
			_, err := fmt.Fprintf(os.Stdout, "synced %d\n", next)
			require.NoError(t, err)
		}
	}
}

// appendTornRecord appends a record header whose payload never made it to
// disk to the newest segment, as a crash in the middle of a write would.
func appendTornRecord(t *testing.T, dir string) {
	t.Helper()
	segs, err := filepath.Glob(filepath.Join(dir, "*.wal"))
	require.NoError(t, err)
	require.NotEmpty(t, segs)
	slices.Sort(segs)
	f, err := os.OpenFile(segs[len(segs)-1], os.O_WRONLY|os.O_APPEND, 0)
	require.NoError(t, err)
	var hdr [8]byte
	binary.LittleEndian.PutUint32(hdr[:4], 64)
	binary.LittleEndian.PutUint32(hdr[4:], 0xdeadbeef)
	_, err = f.Write(append(hdr[:], "partial payload"...))
	require.NoError(t, err)
	require.NoError(t, f.Close())
}
