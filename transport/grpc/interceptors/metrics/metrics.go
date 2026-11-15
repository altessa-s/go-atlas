// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package metrics

import (
	"context"
	"hash/fnv"
	"math/rand/v2"
	"sync"
	"sync/atomic"
	"time"

	"github.com/altessa-s/go-atlas-core/text/strings/intern"
	"github.com/altessa-s/go-atlas/observability/metrics"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors"

	"google.golang.org/grpc/codes"

	sharedmetadata "github.com/altessa-s/go-atlas/transport/grpc/interceptors/metadata"
	stdGrpc "google.golang.org/grpc"
)

// interceptorName is the name of the metrics interceptor.
const interceptorName = "metrics"

// Name returns the interceptor name used for dependency resolution and chain ordering.
func Name() string { return interceptorName }

// ID is a lightweight [interceptors.Interceptor] reference for this package,
// suitable for passing to exclusion lists.
var ID = interceptors.Ref(interceptorName)

// Ensure serverInterceptorWrapper implements the ServerInterceptor interface.
var _ interceptors.ServerInterceptor = (*serverInterceptorWrapper)(nil)

// Ensure streamWrapper implements grpc.ServerStream interface.
var _ stdGrpc.ServerStream = (*streamWrapper)(nil)

// serverInterceptorWrapper wraps the metrics interceptor with
// instance-specific configuration (ignore checker, options).
type serverInterceptorWrapper struct {
	interceptors.BaseInterceptor
	*interceptor
	opts *options
}

// ServerUnaryInterceptor returns a new unary server interceptor that collects metrics.
func (w *serverInterceptorWrapper) ServerUnaryInterceptor() stdGrpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *stdGrpc.UnaryServerInfo, handler stdGrpc.UnaryHandler) (resp any, err error) {
		_, meta := sharedmetadata.EnsureInContext(ctx, info.FullMethod, info)
		method := intern.String(meta.FullyMethodName)

		if w.ShouldIgnore(method) {
			w.LogIgnored(ctx, method)
			return handler(ctx, req)
		}

		mh := w.methodFor(method)
		w.requestsInFlight.Inc()
		mh.inFlight.Inc()

		defer func() {
			w.requestsInFlight.Dec()
			mh.inFlight.Dec()
			w.recordMetrics(mh, meta.StartTime, req, resp, err)
		}()

		return handler(ctx, req)
	}
}

// ServerStreamInterceptor returns a new streaming server interceptor that collects metrics.
func (w *serverInterceptorWrapper) ServerStreamInterceptor() stdGrpc.StreamServerInterceptor {
	return func(srv any, stream stdGrpc.ServerStream, info *stdGrpc.StreamServerInfo, handler stdGrpc.StreamHandler) error {
		_, meta := sharedmetadata.EnsureInContext(stream.Context(), info.FullMethod, info)
		method := intern.String(meta.FullyMethodName)

		if w.ShouldIgnore(method) {
			w.LogIgnored(stream.Context(), method)
			return handler(srv, stream)
		}

		mh := w.methodFor(method)
		w.requestsInFlight.Inc()
		mh.inFlight.Inc()

		defer func() {
			w.requestsInFlight.Dec()
			mh.inFlight.Dec()
		}()

		// Wrap the stream to track per-message metrics if streaming metrics are enabled
		wrappedStream := &streamWrapper{
			ServerStream: stream,
			interceptor:  w.interceptor,
			handles:      mh,
			fullMethod:   meta.FullyMethodName,
			statusCode:   codes.OK,
		}

		// Initialize sampling for the stream
		wrappedStream.initializeSampling()

		err := handler(srv, wrappedStream)

		// Update status code for streaming metrics
		wrappedStream.statusCode = getStatusCode(err)
		wrappedStream.finalizeStreamMetrics()

		w.recordMetrics(mh, meta.StartTime, nil, nil, err)
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
	return []string{sharedmetadata.Name()}
}

type interceptor struct {
	opts                     *options
	requestsTotal            metrics.Counter
	requestDuration          metrics.Histogram
	requestsInFlight         metrics.Gauge
	requestsInFlightByMethod metrics.Gauge
	requestSize              metrics.Histogram
	responseSize             metrics.Histogram
	streamMessagesSent       metrics.Counter
	streamMessagesReceived   metrics.Counter
	streamMessageSize        metrics.Histogram
	// byMethod caches bound label handles per method (see metrics_helper.go)
	// so the request path never rebuilds label maps.
	byMethod sync.Map
	// methodCount is the number of methods tracked in byMethod, not counting
	// [OverflowMethod]; it enforces [WithMaxMethods].
	methodCount atomic.Int64
}

// streamWrapper wraps a grpc.ServerStream to intercept SendMsg and RecvMsg calls
// for collecting per-message streaming metrics.
type streamWrapper struct {
	stdGrpc.ServerStream
	interceptor         *interceptor
	handles             *methodHandles
	fullMethod          string
	messagesSent        int64
	messagesReceived    int64
	sampledMessagesSent int64
	sampledMessagesRecv int64
	streamSeed          uint64
	methodHashSent      uint64 // Pre-computed hash seed for "sent" messages
	methodHashRecv      uint64 // Pre-computed hash seed for "recv" messages
	statusCode          codes.Code
	streamSampled       bool
}

// Unwrap returns the underlying stream so driven interceptors can compose
// per-message context snapshots through the metrics wrapper.
func (s *streamWrapper) Unwrap() stdGrpc.ServerStream {
	return s.ServerStream
}

// ServerInterceptor returns a new interceptor that collects metrics for gRPC requests.
//
// Each call constructs a fresh interceptor wired to the provided
// [metrics.Meter]. The underlying adapter deduplicates metric
// registrations by name, so reusing the same collector across multiple
// calls with the same subsystem is safe and shares the same metric
// vectors. To emit metrics for two unrelated upstreams against a single
// registry, pass distinct subsystems via [WithMetricsSubsystem].
func ServerInterceptor(opt ...Option) interceptors.ServerInterceptor {
	opts := newOptions(opt...)

	i := &interceptor{opts: opts}
	i.initializeMetrics()

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

// ServerUnaryInterceptor returns a new unary server interceptor that collects metrics.
func ServerUnaryInterceptor(opt ...Option) stdGrpc.UnaryServerInterceptor {
	return ServerInterceptor(opt...).ServerUnaryInterceptor()
}

// ServerStreamInterceptor returns a new streaming server interceptor that collects metrics.
func ServerStreamInterceptor(opt ...Option) stdGrpc.StreamServerInterceptor {
	return ServerInterceptor(opt...).ServerStreamInterceptor()
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
		// #nosec G404 -- math/rand intentional for sampling
		rng := rand.New(rand.NewPCG(s.streamSeed, 0))
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
		if s.interceptor.opts.enableSizeMetrics && s.handles.msgSizeSent != nil {
			if size, ok := getMessageSize(m); ok {
				s.handles.msgSizeSent.Observe(float64(size))
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
		if s.interceptor.opts.enableSizeMetrics && s.handles.msgSizeRecv != nil {
			if size, ok := getMessageSize(m); ok {
				s.handles.msgSizeRecv.Observe(float64(size))
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

	// For PerMessageSampling, make an individual decision per message,
	// deterministic per (method+direction, stream, message index).
	var baseHash uint64
	if isSent {
		baseHash = s.methodHashSent
	} else {
		baseHash = s.methodHashRecv
	}

	// Walk a SplitMix64 sequence seeded by the method hash and stream seed:
	// the finalizer avalanches every input bit, so consecutive message
	// indices yield independent decisions. A plain XOR with the index would
	// only flip low bits and leave the whole stream on the same side of the
	// threshold.
	finalHash := splitmix64((baseHash ^ s.streamSeed) + uint64(messageCount)*splitmixGamma) // #nosec G115 -- messageCount is non-negative

	// The top 53 bits map exactly onto a float64 in [0, 1).
	hashValue := float64(finalHash>>(uint64Bits-float64Mantissa)) / (1 << float64Mantissa)
	return hashValue < samplingRate
}

// SplitMix64 constants (Steele, Lea, Flood 2014).
const (
	splitmixGamma   = 0x9E3779B97F4A7C15 // Increment: 2^64 / golden ratio, odd.
	splitmixMul1    = 0xBF58476D1CE4E5B9
	splitmixMul2    = 0x94D049BB133111EB
	splitmixShift1  = 30
	splitmixShift2  = 27
	splitmixShift3  = 31
	float64Mantissa = 53 // Bits of precision in a float64 significand.
	uint64Bits      = 64
)

// splitmix64 is the SplitMix64 output finalizer: a cheap bijective mix whose
// every output bit depends on every input bit.
func splitmix64(x uint64) uint64 {
	x = (x ^ (x >> splitmixShift1)) * splitmixMul1
	x = (x ^ (x >> splitmixShift2)) * splitmixMul2
	return x ^ (x >> splitmixShift3)
}

// finalizeStreamMetrics records the final stream metrics when the stream completes.
func (s *streamWrapper) finalizeStreamMetrics() {
	// If streaming metrics are disabled, don't record any metrics
	if !s.interceptor.opts.enableStreamMetrics {
		return
	}

	samplingRate := s.interceptor.opts.streamSamplingRate
	internedStatus := intern.String(s.statusCode.String())

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
		s.interceptor.statusFor(s.handles, internedStatus).streamSent.Add(scaledSent)
	}

	if s.interceptor.streamMessagesReceived != nil && scaledReceived > 0 {
		s.interceptor.statusFor(s.handles, internedStatus).streamRecv.Add(scaledReceived)
	}
}

func (i *interceptor) initializeMetrics() {
	scoped := i.opts.meter.WithSubsystem(i.opts.metricsSubsystem)

	i.requestsTotal = scoped.MustCounter(metrics.MetricOpts{
		Name:       "server_requests_total",
		Help:       "Total number of gRPC server requests with method and status labels",
		LabelNames: []string{methodLabel, statusLabel},
	})

	i.requestDuration = scoped.MustHistogram(metrics.HistogramOpts{
		Name:       "server_request_duration_seconds",
		Help:       "Duration of gRPC server requests in seconds",
		LabelNames: []string{methodLabel, statusLabel},
		Buckets:    i.opts.durationBuckets,
	})

	i.requestsInFlight = scoped.MustGauge(metrics.MetricOpts{
		Name: "server_requests_in_flight",
		Help: "Current number of concurrent gRPC server requests being processed",
	})

	i.requestsInFlightByMethod = scoped.MustGauge(metrics.MetricOpts{
		Name:       "server_requests_in_flight_by_method",
		Help:       "Current number of concurrent gRPC server requests being processed by method",
		LabelNames: []string{methodLabel},
	})

	if i.opts.enableSizeMetrics {
		i.requestSize = scoped.MustHistogram(metrics.HistogramOpts{
			Name:       "server_request_size_bytes",
			Help:       "Size of gRPC server request messages in bytes",
			LabelNames: []string{methodLabel, statusLabel},
			Buckets:    i.opts.sizeBuckets,
		})

		i.responseSize = scoped.MustHistogram(metrics.HistogramOpts{
			Name:       "server_response_size_bytes",
			Help:       "Size of gRPC server response messages in bytes",
			LabelNames: []string{methodLabel, statusLabel},
			Buckets:    i.opts.sizeBuckets,
		})
	}

	if i.opts.enableStreamMetrics {
		i.streamMessagesSent = scoped.MustCounter(metrics.MetricOpts{
			Name:       "server_stream_messages_sent_total",
			Help:       "Total number of messages sent by server in streaming responses",
			LabelNames: []string{methodLabel, statusLabel},
		})

		i.streamMessagesReceived = scoped.MustCounter(metrics.MetricOpts{
			Name:       "server_stream_messages_received_total",
			Help:       "Total number of messages received from client in streaming requests",
			LabelNames: []string{methodLabel, statusLabel},
		})

		// Stream message size metric (only if both stream and size metrics are enabled)
		if i.opts.enableSizeMetrics {
			i.streamMessageSize = scoped.MustHistogram(metrics.HistogramOpts{
				Name:       "server_stream_message_size_bytes",
				Help:       "Size of individual gRPC stream messages in bytes",
				LabelNames: []string{methodLabel, directionLabel},
				Buckets:    i.opts.sizeBuckets,
			})
		}
	}
}
