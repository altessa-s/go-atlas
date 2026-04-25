// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package msg defines the message envelope for broker systems.
//
// [Message] carries a topic, payload, [Meta] metadata, TTL, and an [Acker]
// for acknowledgment. Use [NewMessage] or [NewMessageWithMeta] to create
// messages. After processing:
//
//   - [Message.Ack] — positive acknowledgment (successful processing)
//   - [Message.Nak] — negative acknowledgment (retry with optional delay)
//   - [Message.Term] — terminal acknowledgment (discard, no redelivery)
//   - [Message.InProgress] — heartbeat to prevent premature redelivery
//
// # Example
//
//	m := msg.NewMessage("events", data)
//	// After processing:
//	m.Ack()   // success
//	m.Nak()   // retry
//	m.Term()  // discard
package msg
