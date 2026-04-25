// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package prometheus

import (
	"context"
	"sync"
	"time"

	"github.com/altessa-s/go-atlas/transport/grpc/interceptors"

	sharedmetadata "github.com/altessa-s/go-atlas/transport/grpc/interceptors/metadata"
	prom "github.com/prometheus/client_golang/prometheus"
	stdGrpc "google.golang.org/grpc"
)

// serverMetricsSingleton holds the singleton instance for server metrics.
// This ensures that Prometheus metrics are registered only once per process,
// preventing "already registered" errors when ServerInterceptor is called multiple times.
var (
	serverMetricsOnce     sync.Once
	serverMetricsInstance *interceptor
)

// getOrCreateServerMetrics returns the singleton server metrics instance.
// On first call, it initializes and registers all Prometheus metrics.
// On subsequent calls, it returns the cached instance.
//
// Note: This function uses the options from the first call to configure metrics.
// If different options are needed, use the registerer option to register
// metrics with a custom registry.
func getOrCreateServerMetrics(opts *options) *interceptor {
	serverMetricsOnce.Do(func() {
		serverMetricsInstance = &interceptor{
			opts: opts,
		}
		serverMetricsInstance.initializeMetrics()
		serverMetricsInstance.registerMetrics()
	})

	return serverMetricsInstance
}

// clientMetrics holds the client-side Prometheus metrics.
type clientMetrics struct {
	opts                  *options
	clientRequestsTotal   *prom.CounterVec
	clientRequestDuration *prom.HistogramVec
	clientRequestSize     *prom.HistogramVec
	clientResponseSize    *prom.HistogramVec
}

// clientMetricsSingleton holds the singleton instance for client metrics.
var (
	clientMetricsOnce     sync.Once
	clientMetricsInstance *clientMetrics
)

// initClientMetrics initializes the client-side Prometheus metrics.
func initClientMetrics(opts *options) *clientMetrics {
	cm := &clientMetrics{opts: opts}

	cm.clientRequestsTotal = prom.NewCounterVec(
		prom.CounterOpts{
			Namespace: opts.namespace,
			Subsystem: opts.subsystem,
			Name:      "client_requests_total",
			Help:      "Total number of gRPC client requests with method and status labels",
		}, []string{methodLabel, statusLabel})

	cm.clientRequestDuration = prom.NewHistogramVec(
		prom.HistogramOpts{
			Namespace: opts.namespace,
			Subsystem: opts.subsystem,
			Name:      "client_request_duration_seconds",
			Help:      "Duration of gRPC client requests in seconds",
			Buckets:   opts.durationBuckets,
		}, []string{methodLabel, statusLabel})

	// Optional size metrics
	if opts.enableSizeMetrics {
		cm.clientRequestSize = prom.NewHistogramVec(
			prom.HistogramOpts{
				Namespace: opts.namespace,
				Subsystem: opts.subsystem,
				Name:      "client_request_size_bytes",
				Help:      "Size of gRPC client request messages in bytes",
				Buckets:   opts.sizeBuckets,
			}, []string{methodLabel, statusLabel})

		cm.clientResponseSize = prom.NewHistogramVec(
			prom.HistogramOpts{
				Namespace: opts.namespace,
				Subsystem: opts.subsystem,
				Name:      "client_response_size_bytes",
				Help:      "Size of gRPC client response messages in bytes",
				Buckets:   opts.sizeBuckets,
			}, []string{methodLabel, statusLabel})
	}

	// Register metrics
	collectors := []prom.Collector{cm.clientRequestsTotal, cm.clientRequestDuration}
	if opts.enableSizeMetrics {
		collectors = append(collectors, cm.clientRequestSize, cm.clientResponseSize)
	}

	for _, c := range collectors {
		_ = opts.registerer.Register(c) //nolint:errcheck // Ignore AlreadyRegisteredError for singleton pattern
	}

	return cm
}

// getOrCreateClientMetrics returns the singleton client metrics instance.
func getOrCreateClientMetrics(opts *options) *clientInterceptor {
	clientMetricsOnce.Do(func() {
		clientMetricsInstance = initClientMetrics(opts)
	})

	return &clientInterceptor{
		clientMetrics: clientMetricsInstance,
		opts:          opts,
	}
}

var _ interceptors.ClientInterceptor = (*clientInterceptor)(nil)

type clientInterceptor struct {
	*clientMetrics
	opts *options
}

func (i *clientInterceptor) Name() string {
	return interceptorName
}

func (i *clientInterceptor) ClientUnaryInterceptor() stdGrpc.UnaryClientInterceptor {
	return func(
		ctx context.Context,
		method string,
		req, reply any,
		cc *stdGrpc.ClientConn,
		invoker stdGrpc.UnaryInvoker,
		opts ...stdGrpc.CallOption,
	) (err error) {
		startTime := time.Now()
		_, meta := sharedmetadata.EnsureInContextFromMethod(ctx, method)

		defer func() {
			i.recordClientMetrics(meta.FullyMethodName, startTime, req, reply, err)
		}()

		return invoker(ctx, method, req, reply, cc, opts...)
	}
}

func (i *clientInterceptor) ClientStreamInterceptor() stdGrpc.StreamClientInterceptor {
	return func(
		ctx context.Context,
		desc *stdGrpc.StreamDesc,
		cc *stdGrpc.ClientConn,
		method string,
		streamer stdGrpc.Streamer,
		opts ...stdGrpc.CallOption,
	) (stream stdGrpc.ClientStream, err error) {
		startTime := time.Now()
		_, meta := sharedmetadata.EnsureInContextFromMethod(ctx, method)

		defer func() {
			// For streaming, record metrics after the stream is established or fails
			i.recordClientMetrics(meta.FullyMethodName, startTime, nil, nil, err)
		}()

		return streamer(ctx, desc, cc, method, opts...)
	}
}

func (i *clientInterceptor) recordClientMetrics(fullMethod string, startTime time.Time, req, resp any, err error) {
	recorder := &metricsRecorder{
		requestsTotal:   i.clientRequestsTotal,
		requestDuration: i.clientRequestDuration,
		requestSize:     i.clientRequestSize,
		responseSize:    i.clientResponseSize,
		opts:            i.opts,
		logPrefix:       "client ",
	}
	recorder.record(fullMethod, startTime, req, resp, err)
}
