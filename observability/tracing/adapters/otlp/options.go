// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package otlp

//go:generate go tool optgen generate --type=options

import (
	"net/http"
	"time"

	"github.com/altessa-s/go-atlas/observability/tracing/adapters"

	"google.golang.org/grpc"
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
	compression bool `optgen:"default=DefaultCompression" optval:"param"`
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
	// grpcConn is the connection the gRPC exporter sends on. The caller owns
	// it: Shutdown leaves it open. When nil, the adapter dials endpoint with
	// grpc-go defaults and closes that connection on Shutdown. The tracing
	// factory injects a transport/grpc/client connection carrying the
	// configured proxy policy. Has no effect when Protocol is HTTP.
	grpcConn grpc.ClientConnInterface `opt:"GRPCConn"`
	// httpClient is the client the HTTP exporter sends with. When nil, the
	// otlptracehttp default client is used (environment proxies honored).
	// The tracing factory injects a net/http/client client carrying
	// the configured proxy policy. Has no effect when Protocol is gRPC.
	httpClient *http.Client `opt:"HTTPClient"`
}
