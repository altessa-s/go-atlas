// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package otlp

import (
	"context"
	"crypto/tls"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/altessa-s/go-atlas-core/collections/slices"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/encoding/gzip"
	"google.golang.org/grpc/metadata"

	corecontext "github.com/altessa-s/go-atlas-core/context"
	coreerrs "github.com/altessa-s/go-atlas-core/errors"
	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
)

// grpcClient implements otlptrace.Client over a gRPC connection. The
// connection is either injected with [WithGRPCConn] and owned by the
// caller, or dialed by Start and closed by Stop.
type grpcClient struct {
	mu sync.RWMutex

	// injected is the caller-owned connection; nil means Start dials one.
	injected grpc.ClientConnInterface
	// owned is the connection dialed by Start, closed by Stop.
	owned *grpc.ClientConn
	trace coltracepb.TraceServiceClient

	endpoint      string
	md            metadata.MD
	exportTimeout time.Duration

	insecure    bool
	compression bool
	started     bool
}

// newGRPCClient creates a grpcClient from the adapter options.
// No connection is made until Start is called.
func newGRPCClient(cfg *options) *grpcClient {
	return &grpcClient{
		injected:      cfg.grpcConn,
		endpoint:      cfg.endpoint,
		insecure:      cfg.insecure,
		md:            buildExporterMetadata(cfg.headers),
		compression:   cfg.compression,
		exportTimeout: cfg.exportTimeout,
	}
}

// buildExporterMetadata builds an immutable gRPC metadata map with lowercased keys.
func buildExporterMetadata(src map[string]string) metadata.MD {
	if len(src) == 0 {
		return nil
	}
	dst := make(metadata.MD, len(src))
	for k, v := range src {
		dst[strings.ToLower(k)] = []string{v}
	}
	return dst
}

// Start binds the trace service client to the injected connection, or
// dials the endpoint with grpc-go defaults (TLS 1.2+ unless insecure,
// HTTPS_PROXY / NO_PROXY from the environment).
// This implements the otlptrace.Client interface.
func (c *grpcClient) Start(_ context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.started {
		return nil
	}

	conn := c.injected
	if conn == nil {
		creds := credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS12})
		if c.insecure {
			creds = insecure.NewCredentials()
		}
		owned, err := grpc.NewClient(c.endpoint, grpc.WithTransportCredentials(creds))
		if err != nil {
			return coreerrs.WrapOperation(err, "create gRPC client")
		}
		c.owned = owned
		conn = owned
	}

	c.trace = coltracepb.NewTraceServiceClient(conn)
	c.started = true

	return nil
}

// Stop closes the connection dialed by Start. An injected connection is
// left open for its owner.
// This implements the otlptrace.Client interface.
func (c *grpcClient) Stop(_ context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.started {
		return nil
	}

	c.started = false

	if c.owned != nil {
		err := c.owned.Close()
		c.owned = nil
		return err
	}

	return nil
}

// UploadTraces sends trace data to the OTLP collector and returns any rejected span count.
func (c *grpcClient) UploadTraces(ctx context.Context, protoSpans []*tracepb.ResourceSpans) (int64, error) {
	c.mu.RLock()
	if !c.started {
		c.mu.RUnlock()
		return 0, fmt.Errorf("client not started")
	}
	trace := c.trace
	c.mu.RUnlock()

	// Apply export timeout
	if c.exportTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = corecontext.WithMaxTimeout(ctx, c.exportTimeout)
		defer cancel()
	}

	req := &coltracepb.ExportTraceServiceRequest{
		ResourceSpans: protoSpans,
	}

	resp, err := trace.Export(c.injectHeaders(ctx), req, c.buildCallOptions()...)
	if err != nil {
		return 0, coreerrs.WrapOperation(err, "export traces")
	}

	var rejected int64
	if ps := resp.GetPartialSuccess(); ps != nil && ps.GetRejectedSpans() > 0 {
		rejected = ps.GetRejectedSpans()
	}
	return rejected, nil
}

// buildCallOptions builds gRPC call options based on configuration.
func (c *grpcClient) buildCallOptions() []grpc.CallOption {
	var opts []grpc.CallOption

	opts = slices.AppendIf(opts, c.compression, grpc.UseCompressor(gzip.Name))

	return opts
}

// injectHeaders builds a fresh outgoing metadata context containing only the
// configured exporter headers, keeping ctx's deadline and cancellation.
func (c *grpcClient) injectHeaders(ctx context.Context) context.Context {
	return metadata.NewOutgoingContext(ctx, c.md)
}
