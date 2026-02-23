// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package grpc provides gRPC interceptors for automatic request auditing.
package grpc

import (
	"context"
	"strings"
	"time"

	"github.com/altessa-s/go-atlas/data/audit"
	"github.com/altessa-s/go-atlas/observability/tracing"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/driver"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/metadata"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var _ driver.DrivenInterceptor = (*interceptor)(nil)
var _ interceptors.Interceptor = (*interceptor)(nil)

type interceptor struct {
	auditor *audit.Auditor
	opts    *options

	ignoreMethodSet map[string]struct{}
}

// ServerInterceptor returns a gRPC server interceptor that audits calls.
func ServerInterceptor(auditor *audit.Auditor, opts ...Option) interceptors.ServerInterceptor {
	i := newInterceptor(auditor, opts...)
	return interceptors.ServerDrivenInterceptor(i)
}

func newInterceptor(auditor *audit.Auditor, opts ...Option) *interceptor {
	o := newOptions(opts...)
	i := &interceptor{
		auditor:         auditor,
		opts:            o,
		ignoreMethodSet: make(map[string]struct{}, len(o.ignoreMethods)),
	}
	for _, m := range o.ignoreMethods {
		i.ignoreMethodSet[strings.ToLower(m)] = struct{}{}
	}
	return i
}

func (i *interceptor) Name() string {
	return "audit"
}

// Dependencies returns interceptors that audit reads from context.
func (i *interceptor) Dependencies() []string {
	return []string{"metadata", "requestid", "realip", "tracing"}
}

func (i *interceptor) DrivenInterceptor(ctx context.Context) (driver.Driver, context.Context) {
	meta, _ := metadata.FromContext(ctx)

	// Check if method should be ignored.
	if meta != nil {
		method := strings.ToLower(meta.FullyMethodName)
		if _, ignored := i.ignoreMethodSet[method]; ignored {
			return driver.NoopDriver(), ctx
		}
		for _, p := range i.opts.ignorePatterns {
			if p.MatchString(method) {
				return driver.NoopDriver(), ctx
			}
		}
	}

	return &requestDriver{
		interceptor: i,
		meta:        meta,
		ctx:         ctx,
		startTime:   time.Now(),
	}, ctx
}

type requestDriver struct {
	*interceptor
	meta      *metadata.CallMetadata
	ctx       context.Context
	startTime time.Time
}

func (d *requestDriver) PreCall(_ context.Context, _ any) (any, error) {
	return nil, nil //nolint:nilnil
}

func (d *requestDriver) PostCall(ctx context.Context, _ any, err error) error {
	if d.auditor == nil {
		return err
	}

	duration := time.Since(d.startTime)

	var actor audit.Actor
	if d.opts.actorExtractor != nil {
		actor = d.opts.actorExtractor(ctx)
	}

	var resourceType, resourcePath string
	if d.meta != nil {
		resourceType = d.meta.ServiceName
		resourcePath = d.meta.FullyMethodName
	}

	result := audit.Result{Status: audit.ResultStatusSuccess}
	if err != nil {
		st := status.Convert(err)
		result.Code = int(st.Code())
		result.Message = st.Message()
		if st.Code() == codes.PermissionDenied || st.Code() == codes.Unauthenticated {
			result.Status = audit.ResultStatusDenied
		} else {
			result.Status = audit.ResultStatusError
		}
	}

	eventCtx := audit.EventContext{
		TraceID: tracing.TraceIDFromContext(ctx),
		SpanID:  tracing.SpanIDFromContext(ctx),
	}
	if d.opts.requestIDExtractor != nil {
		eventCtx.RequestID = d.opts.requestIDExtractor(ctx)
	}

	event := &audit.Event{
		Type:   audit.EventTypeAPIRequest,
		Action: audit.ActionExecute,
		Actor:  actor,
		Resource: audit.Resource{
			Type: resourceType,
			Path: resourcePath,
		},
		Result:    result,
		Context:   eventCtx,
		Duration:  duration,
		Timestamp: d.startTime,
	}

	d.auditor.Emit(event)

	return err
}
