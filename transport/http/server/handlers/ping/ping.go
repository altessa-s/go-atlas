// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package ping

import (
	"github.com/altessa-s/go-atlas/transport/http/server/writer"
)

// Response is the response shape for the [Handler] endpoint.
type Response struct {
	Message string `json:"message"`
}

// Handler responds with a JSON [Response] containing message "pong".
// Useful for basic connectivity checks where a tiny static endpoint is
// preferable to consulting a full health coordinator.
func Handler(rw writer.ReadWriter) {
	_ = rw.Write(Response{Message: "pong"}) //nolint:errcheck // Best effort
}
