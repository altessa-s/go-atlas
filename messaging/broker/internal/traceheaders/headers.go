// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package traceheaders propagates request trace context through broker messages.
package traceheaders

import (
	"github.com/altessa-s/go-atlas/messaging/broker/msg"
	"github.com/altessa-s/go-atlas/observability/tracing/propagation"
)

// Inject copies trace headers into metadata unless the message supplies its own parent.
func Inject(meta msg.Meta, headers propagation.MapCarrier) msg.Meta {
	if headers == nil {
		return meta
	}
	if _, present := meta.Value("traceparent"); present {
		return meta
	}
	// Messages and their metadata can be reused by concurrent publishers.
	result := make(msg.Meta, 0, len(meta)+len(headers))
	for _, v := range meta {
		if v.Key != "tracestate" {
			result = append(result, v)
		}
	}
	for key, value := range headers {
		result = append(result, msg.MetaData{Key: key, Value: value})
	}
	return result
}
