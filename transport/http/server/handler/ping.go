// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package handler

import (
	"github.com/altessa-s/go-atlas/transport/http/server/writer"
)

// PingResponse is the response for ping endpoint.
type PingResponse struct {
	Message string `json:"message"`
}

// Ping is a handler that responds with a JSON [PingResponse] containing
// message "pong". Useful for basic connectivity checks.
func Ping(rw writer.ReadWriter) {
	_ = rw.Write(PingResponse{Message: "pong"}) //nolint:errcheck // Best effort
}
