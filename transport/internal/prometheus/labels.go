// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package prometheus

import (
	"github.com/altessa-s/go-atlas/core/text/strings"
)

// Standard Prometheus metric labels (interned for memory efficiency).
// These are pre-computed at package initialization to avoid repeated
// string allocations during metrics recording.
var (
	// MethodLabel is the standard label name for HTTP method or gRPC method.
	MethodLabel = strings.InternString("method")

	// StatusLabel is the standard label name for HTTP status code or gRPC status code.
	StatusLabel = strings.InternString("status")

	// PathLabel is the standard label name for HTTP path.
	PathLabel = strings.InternString("path")

	// DirectionLabel is the standard label name for message direction (sent/received).
	DirectionLabel = strings.InternString("direction")
)

// Direction label values for streaming/bidirectional communication.
var (
	// DirectionSent indicates outgoing messages.
	DirectionSent = strings.InternString("sent")

	// DirectionReceived indicates incoming messages.
	DirectionReceived = strings.InternString("received")
)
