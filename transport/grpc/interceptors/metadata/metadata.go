// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package metadata

import (
	"context"
	"net/netip"
	"strings"
	"sync"
	"time"

	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/driver"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"

	corestrings "github.com/altessa-s/go-atlas/core/text/strings"
)

// CallMetadata provides comprehensive RPC call information for interceptors.
// It is intended to be parsed once per RPC and shared across all interceptors
// in the chain via context. Parsed method names are cached in a process-wide
// [sync.Map] for O(1) subsequent lookups.
//
// CallMetadata values are immutable after creation by [NewCallMetadata] and
// are safe to read concurrently from multiple goroutines.
type CallMetadata struct {
	// StartTime when the RPC began (request creation time).
	StartTime time.Time
	// ServiceName extracted from method path (e.g., "UserService").
	ServiceName string
	// MethodName extracted from method path (e.g., "GetUser").
	MethodName string
	// FullyMethodName as provided by gRPC (e.g., "/UserService/GetUser").
	FullyMethodName string
	// IsStream indicates if the RPC is a streaming RPC.
	IsStream bool
	// IsClient indicates if the metadata was captured on the client side.
	IsClient bool
	// StreamType indicates the streaming direction.
	StreamType driver.StreamType
	// ClientPerIP is the IP address of the peer (server-side only).
	ClientPerIP netip.Addr
	// ClientUserAgent is the user-agent string from incoming metadata.
	ClientUserAgent string
}

// callerConstraint limits types for call metadata extraction.
type callerConstraint interface {
	*grpc.StreamServerInfo | *grpc.UnaryServerInfo | *grpc.StreamDesc
}

// parsedMethod caches the result of parsing a full method name.
type parsedMethod struct {
	service string
	method  string
}

var methodCache sync.Map // Map[string]parsedMethod

// NewCallMetadata creates a new CallMetadata instance from gRPC call information.
// It performs high-performance parsing and string interning.
func NewCallMetadata[T callerConstraint](ctx context.Context, fullMethod string, c T) *CallMetadata {
	if meta, ok := FromContext(ctx); ok && meta.FullyMethodName == fullMethod {
		return meta
	}

	callMetadata := &CallMetadata{
		FullyMethodName: fullMethod,
		IsClient:        true,
	}

	// Use cache for parsed method names to avoid string manipulations in hot path
	if v, ok := methodCache.Load(fullMethod); ok {
		m := v.(parsedMethod) //nolint:errcheck // Type is guaranteed by methodCache.Store
		callMetadata.ServiceName = m.service
		callMetadata.MethodName = m.method
	} else {
		// Slow path: parse and intern
		trimmed := strings.TrimPrefix(fullMethod, "/")
		if i := strings.Index(trimmed, "/"); i >= 0 {
			callMetadata.ServiceName = corestrings.InternString(trimmed[:i])
			callMetadata.MethodName = corestrings.InternString(trimmed[i+1:])
		} else {
			callMetadata.ServiceName = corestrings.InternString(trimmed)
		}

		methodCache.Store(fullMethod, parsedMethod{
			service: callMetadata.ServiceName,
			method:  callMetadata.MethodName,
		})
	}

	if c != nil {
		switch ct := any(c).(type) {
		case *grpc.StreamServerInfo:
			callMetadata.IsStream = true
			callMetadata.StreamType = streamType(ct)
			callMetadata.IsClient = false
			callMetadata.StartTime = time.Now()
		case *grpc.UnaryServerInfo:
			callMetadata.IsClient = false
			callMetadata.StartTime = time.Now()
		case *grpc.StreamDesc:
			callMetadata.IsStream = true
			callMetadata.StreamType = streamType(ct)
			callMetadata.StartTime = time.Now()
		}
	}

	if !callMetadata.IsClient {
		if peerIp, ok := peer.FromContext(ctx); ok {
			if addrPort, err := netip.ParseAddrPort(peerIp.Addr.String()); err == nil {
				callMetadata.ClientPerIP = addrPort.Addr()
			}
		}
	}

	userAgent := metadata.ValueFromIncomingContext(ctx, "user-agent")
	if len(userAgent) > 0 {
		callMetadata.ClientUserAgent = corestrings.InternString(userAgent[0])
	}

	return callMetadata
}

// streamType determines streaming direction from gRPC info.
func streamType[T interface {
	*grpc.StreamServerInfo | *grpc.StreamDesc
}](c T) driver.StreamType {
	if c == nil {
		return driver.StreamTypeNone
	}

	switch d := any(c).(type) {
	case *grpc.StreamServerInfo:
		if d.IsClientStream && !d.IsServerStream {
			return driver.StreamTypeClient
		} else if !d.IsClientStream && d.IsServerStream {
			return driver.StreamTypeServer
		}
	case *grpc.StreamDesc:
		if d.ClientStreams && !d.ServerStreams {
			return driver.StreamTypeClient
		} else if !d.ClientStreams && d.ServerStreams {
			return driver.StreamTypeServer
		}
	}

	return driver.StreamTypeBidi
}

// NewCallMetadataFromMethod creates metadata for client interceptors from a method path.
func NewCallMetadataFromMethod(ctx context.Context, fullMethod string) *CallMetadata {
	return NewCallMetadata[*grpc.StreamDesc](ctx, fullMethod, nil)
}

// Duration returns time elapsed since the start of the RPC call.
func (c *CallMetadata) Duration() time.Duration {
	if c.StartTime.IsZero() {
		return 0
	}
	return time.Since(c.StartTime)
}

type callMetadataKey struct{}

// FromContext returns CallMetadata from context if present.
func FromContext(ctx context.Context) (*CallMetadata, bool) {
	c, ok := ctx.Value(callMetadataKey{}).(*CallMetadata)
	return c, ok
}

// Method returns the FullyMethodName of c, or "" when c is nil. It removes
// the nil check that callers would otherwise sprinkle alongside every
// [FromContext] call.
func (c *CallMetadata) Method() string {
	if c == nil {
		return ""
	}

	return c.FullyMethodName
}

// MethodFromContext returns the FullyMethodName of the [CallMetadata] in ctx,
// or "" when no metadata is attached. Equivalent to
//
//	meta, _ := metadata.FromContext(ctx)
//	method := meta.Method()
//
// collapsed into one call for use in interceptor logging and dispatch paths
// that don't care to distinguish "no metadata" from "metadata without a
// method name".
func MethodFromContext(ctx context.Context) string {
	c, _ := FromContext(ctx)

	return c.Method()
}

// NewContext returns a new context with CallMetadata injected.
func NewContext(ctx context.Context, c *CallMetadata) context.Context {
	return context.WithValue(ctx, callMetadataKey{}, c)
}

// EnsureInContext guarantees that CallMetadata exists in the context.
// If it exists, it returns the existing context and metadata.
// If not, it creates new metadata, injects it into a new context, and returns them.
// This helper simplifies the "get or create" pattern in interceptors.
func EnsureInContext[T callerConstraint](ctx context.Context, fullMethod string, c T) (context.Context, *CallMetadata) {
	if meta, ok := FromContext(ctx); ok && meta.FullyMethodName == fullMethod {
		return ctx, meta
	}
	meta := NewCallMetadata(ctx, fullMethod, c)
	return NewContext(ctx, meta), meta
}

// EnsureInContextFromMethod is a version of EnsureInContext for client interceptors.
func EnsureInContextFromMethod(ctx context.Context, fullMethod string) (context.Context, *CallMetadata) {
	if meta, ok := FromContext(ctx); ok && meta.FullyMethodName == fullMethod {
		return ctx, meta
	}
	meta := NewCallMetadataFromMethod(ctx, fullMethod)
	return NewContext(ctx, meta), meta
}
