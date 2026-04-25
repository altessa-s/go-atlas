// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package otlp

//go:generate go run github.com/altessa-s/go-atlas/tools/codegen/optgen generate --type=options

import (
	"time"

	"github.com/altessa-s/go-atlas/observability/tracing/adapters"

	grpcclient "github.com/altessa-s/go-atlas/transport/grpc/client"
)

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

	// DefaultExportTimeout is the default timeout for exporting traces.
	DefaultExportTimeout = 30 * time.Second
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
	// exportTimeout sets the maximum duration for an export RPC.
	exportTimeout time.Duration `optval:"positive" optgen:"default=DefaultExportTimeout"`
	// retry enables gRPC retry with default configuration.
	retry bool `opt:"-"`
	// retryConfig enables gRPC retry with custom configuration.
	retryConfig *grpcclient.RetryConfig `opt:"-"`
	// grpcClientOptions are extra options forwarded to the underlying
	// transport/grpc/client when Protocol is gRPC. The factory layer uses
	// this to inject a proxy resolver materialized from
	// config.GrpcProxy.ClientOptions. Has no effect when Protocol is HTTP.
	grpcClientOptions []grpcclient.Option `opt:"GRPCClientOptions" optgen:"append"`
}

// WithRetry enables gRPC retry with default configuration.
func WithRetry() Option {
	return func(o *options) {
		o.retry = true
		o.retryConfig = nil
	}
}

// WithRetryConfig enables gRPC retry with custom configuration.
func WithRetryConfig(cfg *grpcclient.RetryConfig) Option {
	return func(o *options) {
		if cfg == nil {
			return
		}
		o.retry = true
		o.retryConfig = cfg
	}
}
