// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package prometheus

import (
	"context"
	"hash/fnv"
	"math/rand"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/altessa-s/go-atlas/core/collections/slices"
	"github.com/altessa-s/go-atlas/core/text/strings"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors"

	"google.golang.org/grpc/codes"

	sharedmetadata "github.com/altessa-s/go-atlas/transport/grpc/interceptors/metadata"
	prominternal "github.com/altessa-s/go-atlas/transport/internal/prometheus"
	stdGrpc "google.golang.org/grpc"
)

// interceptorName is the name of the prometheus interceptor.
const interceptorName = "prometheus"

// Ensure serverInterceptorWrapper implements the ServerInterceptor interface.
var _ interceptors.ServerInterceptor = (*serverInterceptorWrapper)(nil)

// Ensure streamWrapper implements grpc.ServerStream interface.
var _ stdGrpc.ServerStream = (*streamWrapper)(nil)

// serverInterceptorWrapper wraps the singleton metrics interceptor
// with instance-specific configuration (ignore checker, options).
type serverInterceptorWrapper struct {
	interceptors.BaseInterceptor
	*interceptor
	opts *options
}

// ServerUnaryInterceptor returns a new unary server interceptor that collects Prometheus metrics.
func (w *serverInterceptorWrapper) ServerUnaryInterceptor() stdGrpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *stdGrpc.UnaryServerInfo, handler stdGrpc.UnaryHandler) (resp any, err error) {
		_, meta := sharedmetadata.EnsureInContext(ctx, info.FullMethod, info)
		method := w.InternMethod(meta.FullyMethodName)

		if w.ShouldIgnore(method) {
			w.LogIgnored(ctx, method)
			return handler(ctx, req)
		}

		w.interceptor.requestsInFlight.Inc()
		w.interceptor.requestsInFlightByMethod.WithLabelValues(meta.FullyMethodName).Inc()

		defer func() {
			w.interceptor.requestsInFlight.Dec()
			w.interceptor.requestsInFlightByMethod.WithLabelValues(meta.FullyMethodName).Dec()
			w.interceptor.recordMetrics(meta.FullyMethodName, meta.StartTime, req, resp, err)
		}()

		return handler(ctx, req)
	}
}

// ServerStreamInterceptor returns a new streaming server interceptor that collects Prometheus metrics.
func (w *serverInterceptorWrapper) ServerStreamInterceptor() stdGrpc.StreamServerInterceptor {
	return func(srv any, stream stdGrpc.ServerStream, info *stdGrpc.StreamServerInfo, handler stdGrpc.StreamHandler) error {
		_, meta := sharedmetadata.EnsureInContext(stream.Context(), info.FullMethod, info)
		method := w.InternMethod(meta.FullyMethodName)

		if w.ShouldIgnore(method) {
			w.LogIgnored(stream.Context(), method)
			return handler(srv, stream)
		}

		w.interceptor.requestsInFlight.Inc()
		w.interceptor.requestsInFlightByMethod.WithLabelValues(meta.FullyMethodName).Inc()

		defer func() {
			w.interceptor.requestsInFlight.Dec()
			w.interceptor.requestsInFlightByMethod.WithLabelValues(meta.FullyMethodName).Dec()
		}()

		// Wrap the stream to track per-message metrics if streaming metrics are enabled
		wrappedStream := &streamWrapper{
			ServerStream: stream,
			interceptor:  w.interceptor,
			fullMethod:   meta.FullyMethodName,
			statusCode:   codes.OK,
		}

		// Initialize sampling for the stream
		wrappedStream.initializeSampling()

		err := handler(srv, wrappedStream)

		// Update status code for streaming metrics
		wrappedStream.statusCode = getStatusCode(err)
		wrappedStream.finalizeStreamMetrics()

		w.interceptor.recordMetrics(meta.FullyMethodName, meta.StartTime, nil, nil, err)
		return err
	}
}

// Name returns the name of the interceptor (required to resolve ambiguity with embedded BaseInterceptor).
func (w *serverInterceptorWrapper) Name() string {
	return w.BaseInterceptor.Name()
}

// Dependencies returns interceptors that prometheus requires to run before it.
// Prometheus uses metadata for method name extraction.
func (w *serverInterceptorWrapper) Dependencies() []string {
	return []string{"metadata"}
}

type interceptor struct {
	opts                     *options
	requestsTotal            *prometheus.CounterVec
	requestDuration          *prometheus.HistogramVec
	requestsInFlight         prometheus.Gauge
	requestsInFlightByMethod *prometheus.GaugeVec
	requestSize              *prometheus.HistogramVec
	responseSize             *prometheus.HistogramVec
	streamMessagesSent       *prometheus.CounterVec
	streamMessagesReceived   *prometheus.CounterVec
	streamMessageSize        *prometheus.HistogramVec
	registeredMetrics        []prometheus.Collector
}

// streamWrapper wraps a grpc.ServerStream to intercept SendMsg and RecvMsg calls
// for collecting per-message streaming metrics.
type streamWrapper struct {
	stdGrpc.ServerStream
	interceptor         *interceptor
	fullMethod          string
	statusCode          codes.Code
	messagesSent        int64
	messagesReceived    int64
	sampledMessagesSent int64
	sampledMessagesRecv int64
	streamSampled       bool
	streamSeed          uint64
	methodHashSent      uint64 // Pre-computed hash seed for "sent" messages
	methodHashRecv      uint64 // Pre-computed hash seed for "recv" messages
}

// ServerInterceptor returns a new interceptor that collects Prometheus metrics for gRPC requests.
//
// This function uses a singleton pattern to ensure that Prometheus metrics are registered
// only once per process. Calling this function multiple times will return interceptors
// that share the same underlying metrics collectors.
//
// Note: The metrics configuration (namespace, subsystem, buckets) is determined by
// the first call to this function. Subsequent calls with different options will use
// the metrics from the first initialization.
func ServerInterceptor(opt ...Option) interceptors.ServerInterceptor {
	opts := newOptions(opt...)

	// Use singleton pattern to avoid "already registered" errors
	i := getOrCreateServerMetrics(opts)

	// Return a wrapper that uses the shared metrics but has its own ignore checker
	return &serverInterceptorWrapper{
		BaseInterceptor: interceptors.NewBaseInterceptorWithFilter(
			interceptorName,
			opts.ignoreMethods,
			opts.ignorePatterns,
			opts.logger,
		),
		interceptor: i,
		opts:        opts,
	}
}

// ServerUnaryInterceptor returns a new unary server interceptor that collects Prometheus metrics.
func ServerUnaryInterceptor(opt ...Option) stdGrpc.UnaryServerInterceptor {
	return ServerInterceptor(opt...).ServerUnaryInterceptor()
}

// ServerStreamInterceptor returns a new streaming server interceptor that collects Prometheus metrics.
func ServerStreamInterceptor(opt ...Option) stdGrpc.StreamServerInterceptor {
	return ServerInterceptor(opt...).ServerStreamInterceptor()
}

// ClientInterceptor returns a new interceptor that collects Prometheus metrics for gRPC client calls.
//
// This function uses a singleton pattern to ensure that Prometheus metrics are registered
// only once per process. Calling this function multiple times will return interceptors
// that share the same underlying metrics collectors.
//
// Example:
//
//	interceptor := prometheus.ClientInterceptor(
//	    prometheus.WithNamespace("myapp"),
//	    prometheus.WithSubsystem("grpc_client"),
//	    prometheus.WithEnableSizeMetrics(true),
//	)
//	conn, err := grpc.Dial(
//	    target,
//	    grpc.WithUnaryInterceptor(interceptor.ClientUnaryInterceptor()),
//	    grpc.WithStreamInterceptor(interceptor.ClientStreamInterceptor()),
//	)
func ClientInterceptor(opt ...Option) interceptors.ClientInterceptor {
	opts := newOptions(opt...)
	return getOrCreateClientMetrics(opts)
}

// Name returns the name of the interceptor.
func (i *interceptor) Name() string {
	return interceptorName
}

// initializeSampling sets up sampling configuration for the stream.
func (s *streamWrapper) initializeSampling() {
	// If streaming metrics are disabled, skip all sampling setup
	if !s.interceptor.opts.enableStreamMetrics {
		s.streamSampled = false
		return
	}

	samplingRate := s.interceptor.opts.streamSamplingRate

	// If sampling rate is 1.0 (default), no sampling needed
	if samplingRate >= MaxStreamSamplingRate {
		s.streamSampled = true
		return
	}

	// If sampling rate is 0.0, disable all streaming metrics
	if samplingRate <= MinStreamSamplingRate {
		s.streamSampled = false
		return
	}

	// For PerStreamSampling strategy, make a single decision for the entire stream
	if s.interceptor.opts.streamSamplingStrategy == PerStreamSampling {
		// Use method name and timestamp to generate a deterministic seed
		hash := fnv.New64a()
		_, _ = hash.Write([]byte(s.fullMethod))                //nolint:errcheck
		_, _ = hash.Write([]byte{byte(time.Now().UnixNano())}) //nolint:errcheck
		s.streamSeed = hash.Sum64()

		// Use the seed to make a deterministic sampling decision
		// #nosec G404,G115 -- math/rand intentional for sampling; bit pattern reuse is safe
		rng := rand.New(rand.NewSource(int64(s.streamSeed)))
		s.streamSampled = rng.Float64() < samplingRate
	} else {
		// PerMessageSampling: individual message decisions will be made in SendMsg/RecvMsg
		s.streamSampled = true
		s.streamSeed = uint64(time.Now().UnixNano()) // #nosec G115 -- bit pattern reuse for seeding

		// Pre-compute hash seeds for "sent" and "recv" messages to avoid heavy hashing on every message
		hashSent := fnv.New64a()
		_, _ = hashSent.Write([]byte(s.fullMethod)) //nolint:errcheck
		_, _ = hashSent.Write([]byte("sent"))       //nolint:errcheck
		s.methodHashSent = hashSent.Sum64()

		hashRecv := fnv.New64a()
		_, _ = hashRecv.Write([]byte(s.fullMethod)) //nolint:errcheck
		_, _ = hashRecv.Write([]byte("recv"))       //nolint:errcheck
		s.methodHashRecv = hashRecv.Sum64()
	}
}

// SendMsg intercepts the stream's SendMsg calls to track sent messages.
func (s *streamWrapper) SendMsg(m any) error {
	err := s.ServerStream.SendMsg(m)

	// Only track messages if streaming metrics are enabled
	if !s.interceptor.opts.enableStreamMetrics {
		return err
	}

	s.messagesSent++

	// Check if we should record this message based on sampling configuration
	shouldRecord := s.shouldSampleMessage(s.messagesSent, true)

	if shouldRecord {
		s.sampledMessagesSent++

		// Record message size if both stream and size metrics are enabled
		if s.interceptor.opts.enableSizeMetrics && s.interceptor.streamMessageSize != nil {
			if size, ok := prominternal.GetMessageSize(m); ok {
				s.interceptor.streamMessageSize.WithLabelValues(
					s.fullMethod,
					directionSent,
				).Observe(float64(size))
			}
		}
	}

	return err
}

// RecvMsg intercepts the stream's RecvMsg calls to track received messages.
func (s *streamWrapper) RecvMsg(m any) error {
	err := s.ServerStream.RecvMsg(m)
	if err != nil {
		return err
	}

	// Only track messages if streaming metrics are enabled
	if !s.interceptor.opts.enableStreamMetrics {
		return err
	}

	s.messagesReceived++

	// Check if we should record this message based on sampling configuration
	shouldRecord := s.shouldSampleMessage(s.messagesReceived, false)

	if shouldRecord {
		s.sampledMessagesRecv++

		// Record message size if both stream and size metrics are enabled
		if s.interceptor.opts.enableSizeMetrics && s.interceptor.streamMessageSize != nil {
			if size, ok := prominternal.GetMessageSize(m); ok {
				s.interceptor.streamMessageSize.WithLabelValues(
					s.fullMethod,
					directionReceived,
				).Observe(float64(size))
			}
		}
	}

	return nil
}

// shouldSampleMessage determines if a message should be sampled based on the configured strategy.
func (s *streamWrapper) shouldSampleMessage(messageCount int64, isSent bool) bool {
	// If streaming metrics are disabled, never sample
	if !s.interceptor.opts.enableStreamMetrics {
		return false
	}

	samplingRate := s.interceptor.opts.streamSamplingRate

	// If sampling rate is 1.0, always sample
	if samplingRate >= MaxStreamSamplingRate {
		return true
	}

	// If sampling rate is 0.0, never sample
	if samplingRate <= MinStreamSamplingRate {
		return false
	}

	// For PerStreamSampling, use the stream-level decision
	if s.interceptor.opts.streamSamplingStrategy == PerStreamSampling {
		return s.streamSampled
	}

	// For PerMessageSampling, make an individual decision per message
	// Use optimized approach with pre-computed hash seeds and simple message count XOR
	var baseHash uint64
	if isSent {
		baseHash = s.methodHashSent
	} else {
		baseHash = s.methodHashRecv
	}

	// Use XOR with message count for fast deterministic variation per message
	// This is much faster than creating new hash objects and writing bytes
	finalHash := baseHash ^ uint64(messageCount) ^ s.streamSeed // #nosec G115 -- messageCount is non-negative

	// Use the hash to generate a deterministic float between 0 and 1
	hashValue := float64(finalHash) / float64(^uint64(0))
	return hashValue < samplingRate
}

// finalizeStreamMetrics records the final stream metrics when the stream completes.
func (s *streamWrapper) finalizeStreamMetrics() {
	// If streaming metrics are disabled, don't record any metrics
	if !s.interceptor.opts.enableStreamMetrics {
		return
	}

	samplingRate := s.interceptor.opts.streamSamplingRate
	internedStatus := strings.InternString(s.statusCode.String())

	// If sampling is disabled, don't record any metrics
	if samplingRate <= MinStreamSamplingRate {
		return
	}

	// Calculate scaled values based on sampling rate and sampled counts
	var scaledSent, scaledReceived float64

	if samplingRate >= MaxStreamSamplingRate {
		// No sampling, use actual counts
		scaledSent = float64(s.messagesSent)
		scaledReceived = float64(s.messagesReceived)
	} else {
		// Apply scaling to maintain accurate totals
		// Scale by the inverse of sampling rate to compensate for sampling
		scaleFactor := 1.0 / samplingRate
		scaledSent = float64(s.sampledMessagesSent) * scaleFactor
		scaledReceived = float64(s.sampledMessagesRecv) * scaleFactor
	}

	// Only record metrics if the counters are initialized
	if s.interceptor.streamMessagesSent != nil && scaledSent > 0 {
		s.interceptor.streamMessagesSent.WithLabelValues(
			s.fullMethod, // Already interned when stream was created
			internedStatus,
		).Add(scaledSent)
	}

	if s.interceptor.streamMessagesReceived != nil && scaledReceived > 0 {
		s.interceptor.streamMessagesReceived.WithLabelValues(
			s.fullMethod, // Already interned when stream was created
			internedStatus,
		).Add(scaledReceived)
	}
}

func (i *interceptor) initializeMetrics() {
	// Core request metrics
	i.requestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: i.buildMetricName("server_requests_total"),
			Help: "Total number of gRPC server requests with method and status labels",
		}, []string{methodLabel, statusLabel})

	i.requestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    i.buildMetricName("server_request_duration_seconds"),
			Help:    "Duration of gRPC server requests in seconds",
			Buckets: i.opts.durationBuckets,
		}, []string{methodLabel, statusLabel})

	i.requestsInFlight = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: i.buildMetricName("server_requests_in_flight"),
			Help: "Current number of concurrent gRPC server requests being processed",
		})

	i.requestsInFlightByMethod = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: i.buildMetricName("server_requests_in_flight_by_method"),
			Help: "Current number of concurrent gRPC server requests being processed by method",
		}, []string{methodLabel})

	// Optional size metrics
	if i.opts.enableSizeMetrics {
		i.requestSize = prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    i.buildMetricName("server_request_size_bytes"),
				Help:    "Size of gRPC server request messages in bytes",
				Buckets: i.opts.sizeBuckets,
			}, []string{methodLabel, statusLabel})

		i.responseSize = prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    i.buildMetricName("server_response_size_bytes"),
				Help:    "Size of gRPC server response messages in bytes",
				Buckets: i.opts.sizeBuckets,
			}, []string{methodLabel, statusLabel})
	}

	// Optional streaming message metrics (only when enabled)
	if i.opts.enableStreamMetrics {
		i.streamMessagesSent = prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: i.buildMetricName("server_stream_messages_sent_total"),
				Help: "Total number of messages sent by server in streaming responses",
			}, []string{methodLabel, statusLabel})

		i.streamMessagesReceived = prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: i.buildMetricName("server_stream_messages_received_total"),
				Help: "Total number of messages received from client in streaming requests",
			}, []string{methodLabel, statusLabel})

		// Stream message size metric (only if both stream and size metrics are enabled)
		if i.opts.enableSizeMetrics {
			i.streamMessageSize = prometheus.NewHistogramVec(
				prometheus.HistogramOpts{
					Name:    i.buildMetricName("server_stream_message_size_bytes"),
					Help:    "Size of individual gRPC stream messages in bytes",
					Buckets: i.opts.sizeBuckets,
				}, []string{methodLabel, directionLabel})
		}
	}
}

func (i *interceptor) registerMetrics() {
	// Collect all metrics for registration
	collectors := []prometheus.Collector{
		i.requestsTotal,
		i.requestDuration,
		i.requestsInFlight,
		i.requestsInFlightByMethod,
	}

	if i.opts.enableSizeMetrics {
		collectors = append(collectors, i.requestSize, i.responseSize)
	}

	// Only register streaming metrics if enabled
	if i.opts.enableStreamMetrics {
		collectors = append(collectors, i.streamMessagesSent, i.streamMessagesReceived)
		if i.opts.enableSizeMetrics {
			collectors = append(collectors, i.streamMessageSize)
		}
	}

	// Register metrics with the configured registerer using iter.Seq for modern iteration
	for collector := range slices.Values(collectors) {
		_ = i.opts.registerer.Register(collector) //nolint:errcheck // Ignore AlreadyRegisteredError for singleton pattern
	}

	i.registeredMetrics = collectors
}

func (i *interceptor) recordMetrics(fullMethod string, startTime time.Time, req, resp any, err error) {
	recorder := &metricsRecorder{
		requestsTotal:   i.requestsTotal,
		requestDuration: i.requestDuration,
		requestSize:     i.requestSize,
		responseSize:    i.responseSize,
		opts:            i.opts,
		logPrefix:       "",
	}
	recorder.record(fullMethod, startTime, req, resp, err)
}

func (i *interceptor) buildMetricName(suffix string) string {
	return prominternal.BuildMetricNameWithDefault(i.opts.namespace, i.opts.subsystem, suffix, DefaultMetricPrefix)
}
