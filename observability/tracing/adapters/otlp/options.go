// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package otlp

//go:generate go run github.com/altessa-s/go-atlas/tools/codegen/optgen generate --type=options

import "github.com/altessa-s/go-atlas/observability/tracing/adapters"

// Protocol specifies the OTLP transport protocol.
type Protocol string

const (
	// ProtocolGRPC uses gRPC transport.
	ProtocolGRPC Protocol = "grpc"
	// ProtocolHTTP uses HTTP transport.
	ProtocolHTTP Protocol = "http"
)

// Default values for OTLP adapter options.
const (
	// DefaultEndpoint is the default OTLP endpoint.
	// For gRPC, this is typically "host:4317".
	// For HTTP, this is typically "host:4318".
	DefaultEndpoint = "localhost:4317"

	// DefaultProtocol is the default transport protocol.
	DefaultProtocol = ProtocolGRPC

	// DefaultServiceName is the default service name resource attribute.
	DefaultServiceName = "unknown-service"

	// DefaultCompression indicates whether compression is enabled by default.
	DefaultCompression = true
)

// options holds configuration for the OTLP adapter.
type options struct {
	// endpoint sets the OTLP endpoint.
	// For gRPC, this is typically "host:4317".
	// For HTTP, this is typically "host:4318".
	endpoint string `optgen:"default=DefaultEndpoint"`
	// protocol sets the transport protocol.
	protocol Protocol `optgen:"default=DefaultProtocol"`
	// insecure disables TLS.
	// Use this for development or when connecting to a local collector.
	insecure bool
	// headers sets custom headers to send with requests.
	// This can be used for authentication.
	headers map[string]string
	// compression enables or disables gzip compression.
	compression bool `optgen:"default=DefaultCompression"`
	// serviceName sets the service name resource attribute.
	serviceName string `optgen:"default=DefaultServiceName"`
	// serviceVersion sets the service version resource attribute.
	serviceVersion string
	// environment sets the deployment environment resource attribute.
	environment string
	// resourceAttrs adds custom resource attributes.
	resourceAttrs []adapters.Attribute
}
