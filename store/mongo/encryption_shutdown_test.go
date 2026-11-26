// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"context"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/altessa-s/go-atlas/store/mongo/migrations"
)

type shutdownEncryptionClient struct {
	entered, canceled chan string
	release, closed   chan struct{}
}

func (c *shutdownEncryptionClient) wait(ctx context.Context, operation string) error {
	c.entered <- operation
	<-ctx.Done()
	c.canceled <- operation
	<-c.release
	return ctx.Err()
}

func (c *shutdownEncryptionClient) GetKeyByAltName(ctx context.Context, name string) *mongo.SingleResult {
	switch name {
	case "encrypt":
		return keyDoc(bson.Binary{Subtype: bson.TypeBinaryUUID, Data: []byte("0123456789abcdef")})
	case "create":
		return noKeyDoc()
	default:
		return mongo.NewSingleResultFromDocument(bson.D{}, c.wait(ctx, "lookup"), nil)
	}
}

func (c *shutdownEncryptionClient) CreateDataKey(ctx context.Context, _ string, _ ...options.Lister[options.DataKeyOptions]) (bson.Binary, error) {
	return bson.Binary{}, c.wait(ctx, "create")
}

func (c *shutdownEncryptionClient) Encrypt(ctx context.Context, _ bson.RawValue, _ ...options.Lister[options.EncryptOptions]) (bson.Binary, error) {
	return bson.Binary{}, c.wait(ctx, "encrypt")
}

func (c *shutdownEncryptionClient) Close(context.Context) error {
	close(c.closed)
	return nil
}

func TestCloseCancelsInFlightEncryption(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		client := &shutdownEncryptionClient{
			entered: make(chan string, 3), canceled: make(chan string, 3),
			release: make(chan struct{}), closed: make(chan struct{}),
		}
		m := newEncryptingMongo(t, client, WithClientOptions(mockStartupOptions()), WithMigrationRegistry(&migrations.Registry{}))
		provider := m.encryption.provider
		m.encryption.provider = nil // Use the fake instead of opening libmongocrypt.
		require.NoError(t, m.Connect(t.Context()))
		m.encryption.provider = provider
		t.Cleanup(m.Close)
		timedOut := time.After(DefaultCloseTimeout + time.Second)

		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		release := sync.OnceFunc(func() { close(client.release) })
		defer release()
		results := make(chan error, 3)
		go func() {
			_, err := m.Encrypt(ctx, "value", EncryptionAlgDeterministic, "encrypt")
			results <- err
		}()
		go func() {
			_, err := m.DataKey(ctx, "lookup", false)
			results <- err
		}()
		go func() {
			_, err := m.CreateDataKey(ctx, "create")
			results <- err
		}()
		for range 3 {
			select {
			case <-client.entered:
			case <-timedOut:
				t.Error("encryption operations did not reach the fake client")
				return
			}
		}

		closed := make(chan struct{})
		started := time.Now()
		go func() {
			m.Close()
			close(closed)
		}()

		var canceled []string
		for range 3 {
			select {
			case op := <-client.canceled:
				canceled = append(canceled, op)
			case <-timedOut:
				t.Error("Close did not cancel the active encryption operations")
				return
			}
		}
		require.ElementsMatch(t, []string{"encrypt", "lookup", "create"}, canceled)
		require.Equal(t, DefaultCloseTimeout, time.Since(started))
		select {
		case <-closed:
		case <-timedOut:
			t.Error("Close waited beyond its timeout for the client to cooperate")
			return
		}
		select {
		case <-client.closed:
			t.Error("the client closed while an operation was still using it")
		default:
		}
		release()
		for range 3 {
			require.ErrorIs(t, <-results, context.Canceled)
		}
		<-client.closed
	})
}
