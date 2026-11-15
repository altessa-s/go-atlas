// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package natstest starts an in-process NATS server with JetStream and connects clients to it for tests;
// servers and connections are closed through tb.Cleanup.
//
// It lives apart from internal/testhelpers so the heavy server dependencies
// (nats-server, miniredis) stay out of packages that only need generic
// test helpers.
package natstest
