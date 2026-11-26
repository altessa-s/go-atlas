// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"io"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/x/mongo/driver/wiremessage"
)

func TestManagedMongoCloseAfterReconnect(t *testing.T) {
	t.Parallel()
	cfg := baseConfig()
	cfg.Hosts = []string{reconnectEndpoint(t)}
	m, err := New(cfg).Build(t.Context())
	require.NoError(t, err)
	t.Cleanup(m.Mongo.Close)
	t.Cleanup(m.Close)

	require.NoError(t, m.Connect(t.Context()))
	m.Close()
	require.ErrorIs(t, m.Client().Ping(t.Context(), nil), mongo.ErrClientDisconnected)

	require.NoError(t, m.Connect(t.Context()))
	require.NoError(t, m.Client().Ping(t.Context(), nil))
	m.Close()
	require.ErrorIs(t, m.Client().Ping(t.Context(), nil), mongo.ErrClientDisconnected)
}

// Reply with the fields needed by the handshake, build info, and migration
// bookkeeping. The fake database has no applied migrations.
func reconnectEndpoint(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })
	doc, err := bson.Marshal(bson.M{
		"ok": 1.0, "isWritablePrimary": true, "ismaster": true, "maxWireVersion": 21,
		"maxBsonObjectSize": 16 << 20, "maxMessageSizeBytes": 48 << 20, "maxWriteBatchSize": 100000,
		"logicalSessionTimeoutMinutes": 30, "version": "7.0.0", "versionArray": bson.A{7, 0, 0, 0},
		"n": 1, "nModified": 1,
		"cursor": bson.M{"id": int64(0), "ns": "app.migrations", "firstBatch": bson.A{}},
	})
	require.NoError(t, err)
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go serveReconnectClient(conn, doc)
		}
	}()
	return ln.Addr().String()
}

func serveReconnectClient(conn net.Conn, doc []byte) {
	defer func() { _ = conn.Close() }()
	if err := conn.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return
	}
	var header [16]byte
	for {
		if _, err := io.ReadFull(conn, header[:]); err != nil {
			return
		}
		length, id, _, opcode, _, ok := wiremessage.ReadHeader(header[:])
		if !ok || length < 16 {
			return
		}
		if _, err := io.CopyN(io.Discard, conn, int64(length)-16); err != nil {
			return
		}

		var body []byte
		switch opcode {
		case wiremessage.OpQuery:
			opcode = wiremessage.OpReply
			body = wiremessage.AppendReplyFlags(body, 0)
			body = wiremessage.AppendReplyCursorID(body, 0)
			body = wiremessage.AppendReplyStartingFrom(body, 0)
			body = wiremessage.AppendReplyNumberReturned(body, 1)
		case wiremessage.OpMsg:
			body = wiremessage.AppendMsgFlags(body, 0)
			body = wiremessage.AppendMsgSectionType(body, wiremessage.SingleDocument)
		default:
			return
		}
		body = append(body, doc...)
		response := wiremessage.AppendHeader(nil, int32(16+len(body)), 0, id, opcode) // #nosec G115 -- fixed-size test response
		if _, err := conn.Write(append(response, body...)); err != nil {
			return
		}
	}
}
