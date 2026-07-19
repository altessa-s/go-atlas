// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package interceptors

import (
	"log/slog"

	"github.com/altessa-s/go-atlas/core/collections/slices"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/metadata"
	"github.com/altessa-s/go-atlas/transport/internal/depgraph"

	"google.golang.org/grpc"
)

// Chain is a chain of gRPC interceptors that provides automatic
// separation of unary and stream interceptors for both server and client.
// It accepts any type that implements server/client interceptor interfaces.
//
// The chain automatically ensures that MetadataInterceptor is added as the
// first element, providing pre-parsed RPC information to all subsequent
// interceptors in the chain.
//
// Interceptors implementing DependencyDeclarer are automatically ordered
// based on their declared dependencies using topological sort.
type Chain struct {
	list      []any
	nameIndex map[string]struct{} // O(1) lookup for interceptor names
	logger    *slog.Logger
}

// NewChain creates a new chain of gRPC interceptors.
// It accepts any combination of:
//   - Types implementing serverUnaryInterceptor
//   - Types implementing serverStreamInterceptor
//   - Types implementing both server interfaces (ServerInterceptor)
//   - Types implementing clientUnaryInterceptor
//   - Types implementing clientStreamInterceptor
//   - Types implementing both client interfaces (ClientInterceptor)
//
// Invalid types are silently ignored.
func NewChain(i ...any) *Chain {
	c := &Chain{
		nameIndex: make(map[string]struct{}, len(i)),
	}
	for _, v := range i {
		c.add(v)
	}
	return c
}

// Len returns the number of interceptors in the chain.
func (b *Chain) Len() int {
	return len(b.list)
}

// ServerOptions returns gRPC server options that chain all interceptors.
// It automatically separates unary and stream interceptors and returns
// appropriate options for each type. If no interceptors of a particular
// type exist, no option is generated for that type.
//
// Interceptors are automatically ordered based on their declared dependencies.
// MetadataServerInterceptor is automatically prepended if not already present.
// Returns an error if a circular dependency is detected.
func (b *Chain) ServerOptions() ([]grpc.ServerOption, error) {
	list := b.list
	if !b.hasInterceptor("metadata") {
		list = append([]any{ServerDrivenInterceptor(metadata.Interceptor())}, list...)
	}

	// Order interceptors by dependencies
	list, err := orderByDependencies(list, b.logger)
	if err != nil {
		return nil, err
	}

	return buildServerOptions(list), nil
}

// ClientOptions returns gRPC client options that chain all interceptors.
// It automatically separates unary and stream interceptors and returns
// appropriate options for each type. If no interceptors of a particular
// type exist, no option is generated for that type.
//
// Interceptors are automatically ordered based on their declared dependencies.
// MetadataClientInterceptor is automatically prepended if not already present.
// Returns an error if a circular dependency is detected.
func (b *Chain) ClientOptions() ([]grpc.DialOption, error) {
	list := b.list
	if !b.hasInterceptor("metadata") {
		list = append([]any{ClientDrivenInterceptor(metadata.Interceptor())}, list...)
	}

	// Order interceptors by dependencies
	list, err := orderByDependencies(list, b.logger)
	if err != nil {
		return nil, err
	}

	return buildClientOptions(list), nil
}

func (b *Chain) hasInterceptor(name string) bool {
	_, exists := b.nameIndex[name]
	return exists
}

// add validates and adds a single interceptor to the chain.
// Only types implementing server/client interceptor interfaces
// are added; others are silently ignored.
func (b *Chain) add(i any) {
	_, ssok := i.(serverStreamInterceptor)
	_, suok := i.(serverUnaryInterceptor)
	_, csok := i.(clientStreamInterceptor)
	_, cuok := i.(clientUnaryInterceptor)

	if ssok || suok || csok || cuok {
		b.list = append(b.list, i)
		// Update nameIndex for O(1) lookup
		if named, ok := i.(Interceptor); ok {
			b.nameIndex[named.Name()] = struct{}{}
		}
	}
}

// serverUnaryInterceptor is an internal interface for types that provide
// unary server interceptors. This is typically part of the ServerInterceptor
// interface but can be implemented independently.
type serverUnaryInterceptor interface {
	ServerUnaryInterceptor() grpc.UnaryServerInterceptor
}

// serverStreamInterceptor is an internal interface for types that provide
// stream server interceptors. This is typically part of the ServerInterceptor
// interface but can be implemented independently.
type serverStreamInterceptor interface {
	ServerStreamInterceptor() grpc.StreamServerInterceptor
}

// clientUnaryInterceptor is an internal interface for types that provide
// unary client interceptors. This is typically part of the ClientInterceptor
// interface but can be implemented independently.
type clientUnaryInterceptor interface {
	ClientUnaryInterceptor() grpc.UnaryClientInterceptor
}

// clientStreamInterceptor is an internal interface for types that provide
// stream client interceptors. This is typically part of the ClientInterceptor
// interface but can be implemented independently.
type clientStreamInterceptor interface {
	ClientStreamInterceptor() grpc.StreamClientInterceptor
}

// filterByType returns a filter function that checks if an item implements type T
func filterByType[T any](i any) bool {
	_, ok := i.(T)
	return ok
}

// toServerUnary converts an interceptor to grpc.UnaryServerInterceptor
func toServerUnary(i any) grpc.UnaryServerInterceptor {
	return i.(serverUnaryInterceptor).ServerUnaryInterceptor() //nolint:errcheck // filtered above, returns function not error
}

// toServerStream converts an interceptor to grpc.StreamServerInterceptor
func toServerStream(i any) grpc.StreamServerInterceptor {
	return i.(serverStreamInterceptor).ServerStreamInterceptor() //nolint:errcheck // filtered above, returns function not error
}

// toClientUnary converts an interceptor to grpc.UnaryClientInterceptor
func toClientUnary(i any) grpc.UnaryClientInterceptor {
	return i.(clientUnaryInterceptor).ClientUnaryInterceptor() //nolint:errcheck // filtered above, returns function not error
}

// toClientStream converts an interceptor to grpc.StreamClientInterceptor
func toClientStream(i any) grpc.StreamClientInterceptor {
	return i.(clientStreamInterceptor).ClientStreamInterceptor() //nolint:errcheck // filtered above, returns function not error
}

// buildServerOptions builds gRPC server options from a list of interceptors.
func buildServerOptions(list []any) []grpc.ServerOption {
	unaryInterceptors := slices.ToWithFilter(list, filterByType[serverUnaryInterceptor], toServerUnary)
	streamInterceptors := slices.ToWithFilter(list, filterByType[serverStreamInterceptor], toServerStream)

	opts := make([]grpc.ServerOption, 0, 2)
	opts = slices.AppendIf(opts, len(unaryInterceptors) > 0, grpc.ChainUnaryInterceptor(unaryInterceptors...))
	opts = slices.AppendIf(opts, len(streamInterceptors) > 0, grpc.ChainStreamInterceptor(streamInterceptors...))

	return opts
}

// buildClientOptions builds gRPC client options from a list of interceptors.
func buildClientOptions(list []any) []grpc.DialOption {
	unaryInterceptors := slices.ToWithFilter(list, filterByType[clientUnaryInterceptor], toClientUnary)
	streamInterceptors := slices.ToWithFilter(list, filterByType[clientStreamInterceptor], toClientStream)

	opts := make([]grpc.DialOption, 0, 2)
	opts = slices.AppendIf(opts, len(unaryInterceptors) > 0, grpc.WithChainUnaryInterceptor(unaryInterceptors...))
	opts = slices.AppendIf(opts, len(streamInterceptors) > 0, grpc.WithChainStreamInterceptor(streamInterceptors...))

	return opts
}

// WithLogger sets a logger for debug output during dependency resolution.
// When set, missing dependencies are logged at Debug level.
func (b *Chain) WithLogger(logger *slog.Logger) *Chain {
	b.logger = logger
	return b
}

// DependencyOrder returns the computed order of interceptors based on dependencies.
// This is useful for debugging and testing the dependency resolution.
// Returns an error if a circular dependency is detected.
func (b *Chain) DependencyOrder() ([]string, error) {
	list := b.list
	if !b.hasInterceptor("metadata") {
		list = append([]any{ServerDrivenInterceptor(metadata.Interceptor())}, list...)
	}

	sorted, err := orderByDependencies(list, b.logger)
	if err != nil {
		return nil, err
	}

	names := make([]string, 0, len(sorted))
	for _, item := range sorted {
		name := getInterceptorName(item)
		names = slices.AppendIf(names, name != "", name)
	}
	return names, nil
}

// DependencyGraph returns a map of interceptor names to their declared dependencies.
// This is useful for debugging and understanding the dependency structure.
func (b *Chain) DependencyGraph() map[string][]string {
	result := make(map[string][]string)
	for _, item := range b.list {
		name := getInterceptorName(item)
		if name == "" {
			continue
		}

		if declarer, ok := item.(depgraph.DependencyDeclarer); ok {
			result[name] = declarer.Dependencies()
		} else {
			result[name] = nil
		}
	}
	return result
}
