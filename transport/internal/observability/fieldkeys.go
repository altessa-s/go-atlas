// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package observability

import (
	corestrings "github.com/altessa-s/go-atlas/core/text/strings"
	slogx "github.com/altessa-s/go-atlas/observability/slog"
)

// FieldKey is a type alias for slogx.FieldKey, used as the key in
// structured log attributes. It is a plain string underneath but
// aliased for documentation clarity and package-level consistency.
type FieldKey = slogx.FieldKey

// Common field keys shared between HTTP and gRPC transports.
// Interned for memory efficiency - these strings are used frequently
// across many log entries and should be deduplicated in memory.
var (
	// Client information
	FieldKeyClientPeerIP    FieldKey = corestrings.InternString("client.peer_ip")
	FieldKeyClientUserAgent FieldKey = corestrings.InternString("client.user_agent")
	FieldKeyClientRealIP    FieldKey = corestrings.InternString("client.real_ip")

	// Request timing and identification
	FieldKeyRequestID        FieldKey = corestrings.InternString("request.id")
	FieldKeyRequestDeadline  FieldKey = corestrings.InternString("request.deadline")
	FieldKeyRequestStartTime FieldKey = corestrings.InternString("request.start_time")
	FieldKeyRequestEndTime   FieldKey = corestrings.InternString("request.end_time")
	FieldKeyRequestDuration  FieldKey = corestrings.InternString("request.duration")
	FieldKeyRequestContent   FieldKey = corestrings.InternString("request.content")
	FieldKeyResponseContent  FieldKey = corestrings.InternString("response.content")

	// Distributed tracing
	FieldKeyTraceID FieldKey = corestrings.InternString("trace.id")
	FieldKeySpanID  FieldKey = corestrings.InternString("trace.span_id")

	// Error and panic information
	FieldKeyPanic           FieldKey = corestrings.InternString("panic.message")
	FieldKeyPanicStacktrace FieldKey = corestrings.InternString("panic.stacktrace")
	FieldKeyError           FieldKey = corestrings.InternString("error")
)
