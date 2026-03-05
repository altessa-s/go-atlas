// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package otlp

import (
	"context"
	"fmt"
	"sync"

	"github.com/altessa-s/go-atlas/core/collections/slices"

	"google.golang.org/grpc"
	"google.golang.org/grpc/encoding/gzip"
	"google.golang.org/grpc/metadata"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
	grpcclient "github.com/altessa-s/go-atlas/transport/grpc/client"
	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
)

// grpcClient implements otlptrace.Client interface using transport/grpc/client.Client.
// It provides connection management, retry, and logging capabilities from the custom client.
type grpcClient struct {
	// State
	mu sync.RWMutex

	client *grpcclient.Client

	// Configuration
	endpoint string
	headers  map[string]string

	insecure    bool
	compression bool
	started     bool
}

// grpcClientConfig holds configuration for creating a grpcClient.
type grpcClientConfig struct {
	endpoint    string
	insecure    bool
	headers     map[string]string
	compression bool
}

// newGRPCClient creates a new grpcClient with the given configuration.
// The client is not connected until Start() is called.
func newGRPCClient(cfg *grpcClientConfig) *grpcClient {
	return &grpcClient{
		endpoint:    cfg.endpoint,
		insecure:    cfg.insecure,
		headers:     cfg.headers,
		compression: cfg.compression,
	}
}

// Start establishes the connection to the OTLP collector.
// This implements the otlptrace.Client interface.
func (c *grpcClient) Start(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.started {
		return nil
	}

	// Build client options
	opts := []grpcclient.Option{}

	opts = slices.AppendIf(opts, c.insecure, grpcclient.WithInsecure())

	// Create the gRPC client
	client, err := grpcclient.New(ctx, c.endpoint, opts...)
	if err != nil {
		return coreerrs.WrapOperation(err, "create gRPC client")
	}

	c.client = client
	c.started = true

	return nil
}

// Stop closes the connection to the OTLP collector.
// This implements the otlptrace.Client interface.
func (c *grpcClient) Stop(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.started {
		return nil
	}

	c.started = false

	if c.client != nil {
		return c.client.Close(ctx)
	}

	return nil
}

// UploadTraces sends trace data to the OTLP collector.
// This implements the otlptrace.Client interface.
func (c *grpcClient) UploadTraces(ctx context.Context, protoSpans []*tracepb.ResourceSpans) error {
	c.mu.RLock()
	if !c.started {
		c.mu.RUnlock()
		return fmt.Errorf("client not started")
	}
	client := c.client
	c.mu.RUnlock()

	// Get connection from the client
	conn, err := client.GetConnection(ctx)
	if err != nil {
		return coreerrs.WrapOperation(err, "get connection")
	}
	defer client.ReturnConnection(conn)

	// Create TraceService client
	traceClient := coltracepb.NewTraceServiceClient(conn)

	// Build request
	req := &coltracepb.ExportTraceServiceRequest{
		ResourceSpans: protoSpans,
	}

	// Build call options
	callOpts := c.buildCallOptions()

	// Add headers to context as metadata
	ctx = c.injectHeaders(ctx)

	// Export traces
	_, err = traceClient.Export(ctx, req, callOpts...)
	if err != nil {
		return coreerrs.WrapOperation(err, "export traces")
	}

	return nil
}

// buildCallOptions builds gRPC call options based on configuration.
func (c *grpcClient) buildCallOptions() []grpc.CallOption {
	var opts []grpc.CallOption

	opts = slices.AppendIf(opts, c.compression, grpc.UseCompressor(gzip.Name))

	return opts
}

// injectHeaders injects configured headers into the context as gRPC metadata.
func (c *grpcClient) injectHeaders(ctx context.Context) context.Context {
	if len(c.headers) == 0 {
		return ctx
	}

	// Get existing metadata or create new
	md, ok := metadata.FromOutgoingContext(ctx)
	if !ok {
		md = metadata.New(nil)
	} else {
		md = md.Copy()
	}

	// Add headers
	for key, value := range c.headers {
		md.Set(key, value)
	}

	return metadata.NewOutgoingContext(ctx, md)
}
