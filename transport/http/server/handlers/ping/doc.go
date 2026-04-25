// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package ping provides a tiny `/ping` HTTP handler that returns
// `{"message":"pong"}` with HTTP 200. Used for the simplest possible
// connectivity check — when probe traffic must not exercise any
// dependency.
//
// # Usage
//
//	import ping "github.com/altessa-s/go-atlas/transport/http/server/handlers/ping"
//
//	srv.Handle("/internal/ping", ping.Handler)
package ping
