// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package tracing

// SpanKind represents the role of the span in a trace.
type SpanKind int

const (
	// SpanKindUnspecified is the default value when the span kind is not set.
	SpanKindUnspecified SpanKind = iota

	// SpanKindInternal indicates an internal operation within an application.
	SpanKindInternal

	// SpanKindServer indicates a server-side handling of a synchronous RPC or HTTP request.
	SpanKindServer

	// SpanKindClient indicates a client-side handling of a synchronous RPC or HTTP request.
	SpanKindClient

	// SpanKindProducer indicates a producer sending a message to a broker.
	SpanKindProducer

	// SpanKindConsumer indicates a consumer receiving a message from a broker.
	SpanKindConsumer
)

// String returns the string representation of the span kind.
func (k SpanKind) String() string {
	switch k {
	case SpanKindUnspecified:
		return "unspecified"
	case SpanKindInternal:
		return "internal"
	case SpanKindServer:
		return "server"
	case SpanKindClient:
		return "client"
	case SpanKindProducer:
		return "producer"
	case SpanKindConsumer:
		return "consumer"
	default:
		return "unknown"
	}
}
