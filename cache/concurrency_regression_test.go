// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package cache_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/cache"
	"github.com/altessa-s/go-atlas/cache/storages"
	"github.com/altessa-s/go-atlas/cache/storages/freecache"
	"github.com/altessa-s/go-atlas/cache/storages/redis"
	"github.com/altessa-s/go-atlas/internal/testhelpers/servers/redistest"
)

type pausedDecoder struct {
	started chan struct{}
	resume  chan struct{}
}

var errCorrupt = errors.New("corrupt test value")

func (s pausedDecoder) Serialize(value any) ([]byte, error) {
	data := value.([]byte)
	return data, nil
}

func (s pausedDecoder) Deserialize(data []byte, value any) error {
	if string(data) == "corrupt" {
		close(s.started)
		<-s.resume
		// A decoder may alter its input even if it fails. Cleanup still
		// has to compare the original encoded bytes.
		data[0] = 'x'
		return errCorrupt
	}
	*value.(*[]byte) = data
	return nil
}

func TestCorruptDecodeKeepsConcurrentWrite(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name     string
		backends func(t *testing.T) (reader, writer storages.Storage)
	}{
		{name: "shared LRU", backends: func(t *testing.T) (storages.Storage, storages.Storage) {
			backend := newLRU(t)
			return backend, backend
		}},
		{name: "Redis replicas", backends: func(t *testing.T) (storages.Storage, storages.Storage) {
			client, _ := redistest.RedisClient(t)
			return redis.New(client), redis.New(client)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := t.Context()
			readerBackend, writerBackend := tc.backends(t)
			codec := pausedDecoder{started: make(chan struct{}), resume: make(chan struct{})}
			reader := cache.New(readerBackend, cache.WithFailOpen(0), cache.WithSerializer(codec))
			writer := cache.New(writerBackend, cache.WithSerializer(codec))
			require.NoError(t, writer.Save(ctx, "key", []byte("corrupt"), cache.NoTTL))

			readDone := make(chan error, 1)
			go func() {
				var out []byte
				readDone <- reader.Get(ctx, "key", &out)
			}()
			<-codec.started
			writeErr := writer.Save(ctx, "key", []byte("fresh"), cache.NoTTL)
			close(codec.resume)
			require.NoError(t, writeErr)
			require.ErrorIs(t, <-readDone, cache.ErrMissing)
			var out []byte
			require.NoError(t, reader.Get(ctx, "key", &out))
			require.Equal(t, []byte("fresh"), out)
		})
	}
}

type pausedWriter struct {
	storages.Storage
	started chan struct{}
	resume  chan struct{}
}

func (s pausedWriter) Save(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	if string(value) == `"first"` {
		close(s.started)
		select {
		case <-s.resume:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.Storage.Save(ctx, key, value, ttl)
}

func TestMutationDeadlineIncludesWaitForActiveWriter(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		run  func(context.Context, *cache.Cache) error
	}{
		{name: "Save", run: func(ctx context.Context, c *cache.Cache) error { return c.Save(ctx, "key", "second") }},
		{name: "Delete", run: func(ctx context.Context, c *cache.Cache) error { return c.Delete(ctx, "key") }},
		{name: "DeleteMany", run: func(ctx context.Context, c *cache.Cache) error {
			return c.DeleteMany(ctx, "other", "key", "key")
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := pausedWriter{Storage: newLRU(t), started: make(chan struct{}), resume: make(chan struct{})}
			c := cache.New(s)
			writeDone := make(chan error, 1)
			go func() { writeDone <- c.Save(t.Context(), "key", "first") }()
			<-s.started
			waitCtx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
			defer cancel()
			waitDone := make(chan error, 1)
			go func() { waitDone <- tc.run(waitCtx, c) }()
			select {
			case err := <-waitDone:
				close(s.resume)
				require.ErrorIs(t, err, context.DeadlineExceeded)
			case <-time.After(3 * time.Second):
				close(s.resume)
				require.FailNow(t, "mutation did not respect its own deadline")
			}
			require.NoError(t, <-writeDone)
			var got string
			require.NoError(t, c.Get(t.Context(), "key", &got))
			require.Equal(t, "first", got)
		})
	}
}

type aliasingCodec struct{}

func (aliasingCodec) Serialize(value any) ([]byte, error) {
	data := value.([]byte)
	return data, nil
}

func (aliasingCodec) Deserialize(data []byte, value any) error {
	*value.(*[]byte) = data
	return nil
}

func TestAliasingSerializerBufferOwnership(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		storage func(t *testing.T) storages.Storage
	}{
		{name: "LRU", storage: func(t *testing.T) storages.Storage { return newLRU(t) }},
		{name: "FreeCache", storage: func(*testing.T) storages.Storage {
			return freecache.New(freecache.WithMaxSize(1 << 20))
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := t.Context()
			c := cache.New(tc.storage(t), cache.WithIsolatedResults(), cache.WithSerializer(aliasingCodec{}))
			original := []byte{1, 2}
			require.NoError(t, c.Save(ctx, "saved", original))
			original[0] = 3
			var hit []byte
			require.NoError(t, c.Get(ctx, "saved", &hit))
			require.Equal(t, []byte{1, 2}, hit)
			hit[0] = 4
			many, err := cache.GetManyT[[]byte](ctx, c, []string{"saved"})
			require.NoError(t, err)
			require.Equal(t, []byte{1, 2}, many["saved"])
			many["saved"][0] = 5
			require.NoError(t, c.Get(ctx, "saved", &hit))
			require.Equal(t, []byte{1, 2}, hit)

			loaded := []byte{6, 7}
			got, err := cache.GetWithFallbackT(ctx, c, "typed", func(context.Context) ([]byte, time.Duration, error) {
				return loaded, cache.NoTTL, nil
			})
			require.NoError(t, err)
			got[0] = 8
			require.Equal(t, []byte{6, 7}, loaded)
			var untyped []byte
			require.NoError(t, c.GetWithFallback(ctx, "untyped", &untyped, func(context.Context) (any, time.Duration, error) {
				return loaded, cache.NoTTL, nil
			}))
			untyped[0] = 9
			require.Equal(t, []byte{6, 7}, loaded)
			loaded[0] = 10
			for _, key := range []string{"typed", "untyped"} {
				require.NoError(t, c.Get(ctx, key, &hit))
				require.Equal(t, []byte{6, 7}, hit)
			}
		})
	}
}
