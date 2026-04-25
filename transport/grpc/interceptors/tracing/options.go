// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package tracing

//go:generate go run github.com/altessa-s/go-atlas/tools/codegen/optgen generate --type=options

import (
	"log/slog"
	"regexp"

	"github.com/altessa-s/go-atlas/observability/tracing"
	"github.com/altessa-s/go-atlas/observability/tracing/propagation"
)

// SpanNameFunc is a function that generates span names from gRPC method names.
type SpanNameFunc func(fullMethod string) string

// options contains configuration for the tracing interceptor.
type options struct {
	// ignoreMethods sets methods to ignore for tracing.
	// Methods should be full gRPC method names like "/package.Service/Method".
	ignoreMethods []string `optgen:"append"`
	// ignorePatterns sets compiled regex patterns for methods to ignore.
	ignorePatterns []*regexp.Regexp
	// propagator sets the trace context propagator.
	// Defaults to W3C Trace Context propagator.
	propagator propagation.TextMapPropagator `optgen:"default=defaultPropagator()"`
	// spanNameFunc sets a custom function for generating span names.
	// The function receives the full gRPC method and should return the span name.
	spanNameFunc SpanNameFunc `optgen:"default=defaultSpanName"`
	// logger sets the logger for debug messages.
	logger *slog.Logger
}

// defaultPropagator returns the default W3C Trace Context propagator.
func defaultPropagator() propagation.TextMapPropagator {
	return propagation.NewTraceContext()
}

// defaultSpanName returns the full method as the span name.
func defaultSpanName(fullMethod string) string {
	return fullMethod
}

// Semantic convention attributes for gRPC tracing.
const (
	// RPCSystemKey is the key for RPC system attribute.
	RPCSystemKey = "rpc.system"
	// RPCSystemGRPC is the value for gRPC RPC system.
	RPCSystemGRPC = "grpc"
	// RPCServiceKey is the key for RPC service name.
	RPCServiceKey = "rpc.service"
	// RPCMethodKey is the key for RPC method name.
	RPCMethodKey = "rpc.method"
	// RPCGRPCStatusCodeKey is the key for gRPC status code.
	RPCGRPCStatusCodeKey = "rpc.grpc.status_code"
	// NetPeerNameKey is the key for peer name/address.
	NetPeerNameKey = "net.peer.name"
)

// parseMethod splits a full gRPC method into service and method names.
func parseMethod(fullMethod string) (service, method string) {
	// Full method format: /package.Service/Method
	if len(fullMethod) == 0 || fullMethod[0] != '/' {
		return "", fullMethod
	}

	fullMethod = fullMethod[1:] // Remove leading /

	// Find the last / separator
	for i := len(fullMethod) - 1; i >= 0; i-- {
		if fullMethod[i] == '/' {
			return fullMethod[:i], fullMethod[i+1:]
		}
	}

	return "", fullMethod
}

// spanAttributes returns common attributes for gRPC spans.
func spanAttributes(fullMethod string) []tracing.Attribute {
	service, method := parseMethod(fullMethod)
	return []tracing.Attribute{
		tracing.String(RPCSystemKey, RPCSystemGRPC),
		tracing.String(RPCServiceKey, service),
		tracing.String(RPCMethodKey, method),
	}
}
