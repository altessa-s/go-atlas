// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package observability

import (
	"github.com/altessa-s/go-atlas-core/text/strings/intern"
)

// FieldKey is the key of a structured log attribute (slogx.Field.Key). It
// is a plain string, aliased for documentation clarity and package-level
// consistency.
type FieldKey = string

// Common field keys shared between HTTP and gRPC transports.
// Interned for memory efficiency - these strings are used frequently
// across many log entries and should be deduplicated in memory.
var (
	// Client information
	FieldKeyClientPeerIP    FieldKey = intern.String("client.peer_ip")
	FieldKeyClientUserAgent FieldKey = intern.String("client.user_agent")
	FieldKeyClientRealIP    FieldKey = intern.String("client.real_ip")

	// Request timing and identification
	FieldKeyRequestID        FieldKey = intern.String("request.id")
	FieldKeyRequestDeadline  FieldKey = intern.String("request.deadline")
	FieldKeyRequestStartTime FieldKey = intern.String("request.start_time")
	FieldKeyRequestEndTime   FieldKey = intern.String("request.end_time")
	FieldKeyRequestDuration  FieldKey = intern.String("request.duration")
	FieldKeyRequestContent   FieldKey = intern.String("request.content")
	FieldKeyResponseContent  FieldKey = intern.String("response.content")

	// Distributed tracing
	FieldKeyTraceID FieldKey = intern.String("trace.id")
	FieldKeySpanID  FieldKey = intern.String("trace.span_id")

	// Error and panic information
	FieldKeyPanic           FieldKey = intern.String("panic.message")
	FieldKeyPanicStacktrace FieldKey = intern.String("panic.stacktrace")
	FieldKeyError           FieldKey = intern.String("error")
)
