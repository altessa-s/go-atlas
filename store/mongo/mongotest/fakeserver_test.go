// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongotest_test

import (
	"encoding/binary"
	"errors"
	"io"
	"net"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// Wire protocol opcodes the fake server speaks.
const (
	opReply = 1
	opQuery = 2004
	opMsg   = 2013
)

// stallingServer is a minimal MongoDB wire-protocol server: it completes the handshake as a session-capable standalone, fails
// every ping with a command error and never answers endSessions, holding the connection open until the test ends. It is the
// server a readiness failure must not wait on during teardown.
type stallingServer struct {
	tb    testing.TB
	ln    net.Listener
	pings atomic.Int32
}

func newStallingServer(tb testing.TB) *stallingServer {
	tb.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(tb, err)
	s := &stallingServer{tb: tb, ln: ln}
	tb.Cleanup(func() { _ = ln.Close() })
	go s.serve()
	return s
}

func (s *stallingServer) addr() string { return s.ln.Addr().String() }

func (s *stallingServer) serve() {
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			return
		}
		go s.handle(conn)
	}
}

func (s *stallingServer) handle(conn net.Conn) {
	defer func() { _ = conn.Close() }()
	go func() {
		<-s.tb.Context().Done()
		_ = conn.Close()
	}()
	for {
		header := make([]byte, 16)
		if _, err := io.ReadFull(conn, header); err != nil {
			return
		}
		size := int(binary.LittleEndian.Uint32(header))
		requestID := binary.LittleEndian.Uint32(header[4:])
		opcode := binary.LittleEndian.Uint32(header[12:])
		body := make([]byte, size-16)
		if _, err := io.ReadFull(conn, body); err != nil {
			return
		}
		cmd, err := commandOf(opcode, body)
		if err != nil {
			return
		}
		var reply bson.D
		switch cmd {
		case "hello", "isMaster", "ismaster":
			reply = bson.D{
				{Key: "isWritablePrimary", Value: true}, {Key: "ismaster", Value: true},
				{Key: "minWireVersion", Value: int32(0)}, {Key: "maxWireVersion", Value: int32(21)},
				{Key: "logicalSessionTimeoutMinutes", Value: int32(30)},
				{Key: "maxBsonObjectSize", Value: int32(16 << 20)}, {Key: "maxMessageSizeBytes", Value: int32(48 << 20)},
				{Key: "maxWriteBatchSize", Value: int32(100000)}, {Key: "ok", Value: 1.0},
			}
		case "ping":
			s.pings.Add(1)
			reply = bson.D{{Key: "ok", Value: 0.0}, {Key: "errmsg", Value: "injected ping failure"}, {Key: "code", Value: int32(8)}}
		case "endSessions":
			<-s.tb.Context().Done() // never answered
			return
		default:
			reply = bson.D{{Key: "ok", Value: 1.0}}
		}
		if err := writeReply(conn, opcode, requestID, reply); err != nil {
			return
		}
	}
}

// commandOf returns the command name of an OP_QUERY or OP_MSG body.
func commandOf(opcode uint32, body []byte) (string, error) {
	var doc bson.Raw
	switch opcode {
	case opQuery:
		rest := body[4:] // flags
		end := 0
		for end < len(rest) && rest[end] != 0 {
			end++
		}
		doc = bson.Raw(rest[end+1+8:]) // collection name, numberToSkip, numberToReturn
	case opMsg:
		if len(body) < 5 || body[4] != 0 {
			return "", errors.New("unsupported OP_MSG section")
		}
		doc = bson.Raw(body[5:])
	default:
		return "", errors.New("unsupported opcode")
	}
	elems, err := doc.Elements()
	if err != nil || len(elems) == 0 {
		return "", errors.New("malformed command")
	}
	return elems[0].Key(), nil
}

// writeReply answers request requestID in the protocol the request came in.
func writeReply(w io.Writer, opcode, requestID uint32, reply bson.D) error {
	doc, err := bson.Marshal(reply)
	if err != nil {
		return err
	}
	var body []byte
	replyOp := uint32(opMsg)
	if opcode == opQuery {
		replyOp = opReply
		body = binary.LittleEndian.AppendUint32(body, 0) // responseFlags
		body = binary.LittleEndian.AppendUint64(body, 0) // cursorID
		body = binary.LittleEndian.AppendUint32(body, 0) // startingFrom
		body = binary.LittleEndian.AppendUint32(body, 1) // numberReturned
	} else {
		body = binary.LittleEndian.AppendUint32(body, 0) // flagBits
		body = append(body, 0)                           // section kind 0
	}
	body = append(body, doc...)
	header := binary.LittleEndian.AppendUint32(nil, uint32(16+len(body)))
	header = binary.LittleEndian.AppendUint32(header, 0)
	header = binary.LittleEndian.AppendUint32(header, requestID)
	header = binary.LittleEndian.AppendUint32(header, replyOp)
	_, err = w.Write(append(header, body...))
	return err
}
