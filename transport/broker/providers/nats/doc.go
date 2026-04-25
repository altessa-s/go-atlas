// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package natsprovider provides a NATS JetStream implementation of
// [broker.Provider]. It manages persistent streams, consumers, and reliable
// message delivery through the JetStream API.
//
// [Nats] is the main type; create one with [New] or [NewWithContext].
// Use [WithAllowedSubjects] to restrict which subjects may be published or
// subscribed. Attempting to use a disallowed subject returns
// [ErrSubjectNotAllowed].
//
// All exported methods on [Nats] are safe for concurrent use.
//
// # Example
//
//	provider, _ := natsprovider.New(natsConn)
//	b := broker.New(provider)
//	b.Publish(ctx, msg.Message{Topic: "events", Data: data})
package natsprovider
