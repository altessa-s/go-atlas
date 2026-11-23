// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package metrics

import (
	"context"
	"errors"
	"slices"
	"sync"
	"time"

	"github.com/altessa-s/go-atlas/transport/grpc/interceptors"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
)

type clientInterceptor struct {
	interceptors.BaseInterceptor
	metrics *interceptor
}

// ClientInterceptor returns a new interceptor that collects metrics for outgoing gRPC requests.
// Both unary and streaming calls share its method-cardinality cap.
func ClientInterceptor(opt ...Option) interceptors.ClientInterceptor {
	opts := newOptions(opt...)
	i := &interceptor{opts: opts}
	i.initializeMetrics("client")

	return &clientInterceptor{
		BaseInterceptor: interceptors.NewBaseInterceptorWithFilter(
			interceptorName, opts.ignoreMethods, opts.ignorePatterns, opts.logger,
		),
		metrics: i,
	}
}

// ClientUnaryInterceptor returns a new unary client interceptor that collects metrics.
func ClientUnaryInterceptor(opt ...Option) grpc.UnaryClientInterceptor {
	return ClientInterceptor(opt...).ClientUnaryInterceptor()
}

// ClientStreamInterceptor returns a new streaming client interceptor that collects metrics.
func ClientStreamInterceptor(opt ...Option) grpc.StreamClientInterceptor {
	return ClientInterceptor(opt...).ClientStreamInterceptor()
}

func (i *clientInterceptor) ClientUnaryInterceptor() grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoke grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		if i.ShouldIgnore(method) {
			i.LogIgnored(ctx, method)
			return invoke(ctx, method, req, reply, cc, opts...)
		}

		start := time.Now()
		mh := i.metrics.methodFor(method)
		i.metrics.requestsInFlight.Inc()
		mh.inFlight.Inc()
		defer i.metrics.requestsInFlight.Dec()
		defer mh.inFlight.Dec()

		err := invoke(ctx, method, req, reply, cc, opts...)
		if err != nil {
			reply = nil
		}
		i.metrics.recordMetrics(mh, start, req, reply, getClientStatusCode(err))
		return err
	}
}

func (i *clientInterceptor) ClientStreamInterceptor() grpc.StreamClientInterceptor {
	return func(
		ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption,
	) (grpc.ClientStream, error) {
		if i.ShouldIgnore(method) {
			i.LogIgnored(ctx, method)
			return streamer(ctx, desc, cc, method, opts...)
		}

		start := time.Now()
		mh := i.metrics.methodFor(method)
		s := &clientStream{
			start:       start,
			interceptor: i.metrics,
			handles:     mh,
			fullMethod:  method,
		}
		s.initializeSampling()
		i.metrics.requestsInFlight.Inc()
		mh.inFlight.Inc()

		opts = append(slices.Clip(opts), grpc.OnFinish(s.finish))
		stream, err := streamer(ctx, desc, cc, method, opts...)
		if err != nil {
			// A downstream interceptor can reject the stream before it reaches gRPC.
			s.finish(err)
			return stream, err
		}
		if !i.metrics.opts.enableStreamMetrics {
			return stream, nil
		}
		s.ClientStream = stream
		return s, nil
	}
}

type clientStream struct {
	grpc.ClientStream
	streamMetrics
	start time.Time

	mu             sync.Mutex
	activeMessages int
	finished       bool
	code           codes.Code
}

func (s *clientStream) SendMsg(m any) error {
	return s.measureMessage(m, true, s.ClientStream.SendMsg)
}

func (s *clientStream) RecvMsg(m any) error {
	return s.measureMessage(m, false, s.ClientStream.RecvMsg)
}

func (s *clientStream) measureMessage(m any, sent bool, call func(any) error) error {
	s.mu.Lock()
	if s.finished {
		s.mu.Unlock()
		return call(m)
	}
	s.activeMessages++
	s.mu.Unlock()

	err := call(m)

	s.mu.Lock()
	defer s.mu.Unlock()
	if err == nil {
		s.recordMessage(m, sent)
	}
	s.activeMessages--
	if s.finished && s.activeMessages == 0 {
		s.finalizeStreamMetrics(s.code)
	}
	return err
}

func (s *clientStream) finish(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.finished {
		return
	}
	s.finished = true
	s.code = getClientStatusCode(err)

	s.interceptor.requestsInFlight.Dec()
	s.handles.inFlight.Dec()
	s.interceptor.recordMetrics(s.handles, s.start, nil, nil, s.code)

	// gRPC can finish inside RecvMsg before returning the final response.
	// Wait for message accounting before publishing the stream totals.
	if s.activeMessages == 0 {
		s.finalizeStreamMetrics(s.code)
	}
}

func getClientStatusCode(err error) codes.Code {
	if errors.Is(err, context.Canceled) {
		return codes.Canceled
	}
	return getStatusCode(err)
}
