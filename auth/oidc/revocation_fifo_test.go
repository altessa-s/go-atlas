//go:build darwin || linux || freebsd || netbsd || openbsd || dragonfly

// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oidc

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestFileLoaderRejectsWriterlessFIFO(t *testing.T) {
	t.Parallel()
	for _, op := range []string{"count", "stream"} {
		t.Run(op, func(t *testing.T) {
			t.Parallel()
			file := &FileRevocationLoader{Path: filepath.Join(t.TempDir(), "fifo")}
			require.NoError(t, syscall.Mkfifo(file.Path, 0600))
			done := make(chan error, 1)
			ctx := t.Context()
			go func() {
				if op == "count" {
					_, err := file.Count(ctx)
					done <- err
					return
				}
				var streamErr error
				for _, err := range file.StreamValues(ctx) {
					streamErr = err
				}
				done <- streamErr
			}()
			select {
			case err := <-done:
				require.ErrorIs(t, err, ErrRevocationLoadFailed)
			case <-time.After(2 * time.Second):
				writer, err := os.OpenFile(file.Path, os.O_WRONLY|syscall.O_NONBLOCK, 0)
				require.NoError(t, err)
				require.NoError(t, writer.Close())
				<-done
				require.FailNow(t, "opening a writerless FIFO blocked")
			}
		})
	}
}

func TestFileLoaderFIFOReleasesRebuildLease(t *testing.T) {
	t.Parallel()
	client, _ := newSharedBloomRedis(t)
	storage, _ := sharedReplica(client)
	path := filepath.Join(t.TempDir(), "source")
	require.NoError(t, os.WriteFile(path, []byte("first\n"), 0600))
	storage.loader = &FileRevocationLoader{Path: path}
	p := newTestIdP(t).newProvider(t, WithRevocationStorage(storage))
	fifo := path + ".fifo"
	require.NoError(t, syscall.Mkfifo(fifo, 0600))
	require.NoError(t, os.Rename(fifo, path))
	done := make(chan error, 1)
	ctx := t.Context()
	go func() { done <- p.revocation.scheduledRevocationSync(ctx) }()
	select {
	case err := <-done:
		require.ErrorIs(t, err, ErrRevocationLoadFailed)
	case <-time.After(2 * time.Second):
		// Release an old blocking opener and give its second pass a regular file.
		require.NoError(t, os.Rename(path, fifo))
		require.NoError(t, os.WriteFile(path, []byte("restored\n"), 0600))
		writer, err := os.OpenFile(fifo, os.O_RDWR|syscall.O_NONBLOCK, 0)
		require.NoError(t, err)
		require.NoError(t, writer.Close())
		<-done
		require.FailNow(t, "the FIFO retained the rebuild lease")
	}
	peer, _ := sharedReplica(client, "next")
	require.NoError(t, peer.Sync(ctx))
	p.Close()
}
