// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package audit

import (
	"context"
	"time"

	"github.com/altessa-s/go-atlas/data/audit"
	"github.com/altessa-s/go-atlas/observability/tracing"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/driver"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/metadata"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/realip"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/requestid"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	tracinginter "github.com/altessa-s/go-atlas/transport/grpc/interceptors/tracing"
)

const interceptorName = "audit"

// Name returns the interceptor name used for dependency resolution and chain ordering.
func Name() string { return interceptorName }

// ID is a lightweight [interceptors.Interceptor] reference for this package,
// suitable for passing to exclusion lists.
var ID = interceptors.Ref(interceptorName)

var _ driver.DrivenInterceptor = (*interceptor)(nil)
var _ interceptors.Interceptor = (*interceptor)(nil)

type interceptor struct {
	interceptors.BaseInterceptor
	auditor *audit.Auditor
	opts    *options
}

// ServerInterceptor returns a gRPC server interceptor that audits calls.
func ServerInterceptor(auditor *audit.Auditor, opts ...Option) interceptors.ServerInterceptor {
	o := newOptions(opts...)
	i := &interceptor{
		BaseInterceptor: interceptors.NewBaseInterceptorWithFilter(
			interceptorName,
			o.ignoreMethods,
			o.ignorePatterns,
			o.logger,
		),
		auditor: auditor,
		opts:    o,
	}
	return interceptors.ServerDrivenInterceptor(i)
}

// Dependencies returns optional interceptors that audit reads from context.
func (i *interceptor) Dependencies() []string {
	return []string{metadata.Name(), requestid.Name(), tracinginter.Name()}
}

// RequiredDependencies returns interceptors that audit requires to function.
func (i *interceptor) RequiredDependencies() []string {
	return []string{realip.Name()}
}

func (i *interceptor) DrivenInterceptor(ctx context.Context) (driver.Driver, context.Context) {
	meta, ignored := i.ShouldIgnoreFromContext(ctx)
	if ignored {
		return driver.NoopDriver(), ctx
	}

	return &requestDriver{
		interceptor: i,
		meta:        meta,
		startTime:   time.Now(),
	}, ctx
}

type requestDriver struct {
	*interceptor
	meta      *metadata.CallMetadata
	startTime time.Time
}

func (d *requestDriver) PreCall(_ context.Context, _ any) (any, error) {
	return nil, nil //nolint:nilnil
}

func (d *requestDriver) PostCall(ctx context.Context, _ any, err error) error {
	if d.auditor == nil {
		return err
	}

	var actor audit.Actor
	if d.opts.actorExtractor != nil {
		actor = d.opts.actorExtractor(ctx)
	}

	var resourceType, resourcePath string
	if d.meta != nil {
		resourceType = d.meta.ServiceName
		resourcePath = d.meta.FullyMethodName
	}

	info := audit.RequestInfo{
		Action:       audit.ActionExecute,
		ResourceType: resourceType,
		ResourcePath: resourcePath,
		Actor:        actor,
		Context: audit.NewEventContext(
			tracing.TraceIDFromContext(ctx),
			tracing.SpanIDFromContext(ctx),
			requestid.FromContext(ctx),
		),
		StartTime: d.startTime,
		Duration:  time.Since(d.startTime),
	}

	event := audit.BuildTransportEvent(info, func() audit.Result {
		return classifyGRPCStatus(err)
	})

	d.auditor.Emit(event)

	return err
}

// classifyGRPCStatus maps a gRPC error to an audit Result.
func classifyGRPCStatus(err error) audit.Result {
	if err == nil {
		return audit.Result{Status: audit.ResultStatusSuccess}
	}

	st := status.Convert(err)
	result := audit.Result{
		Code:    int(st.Code()),
		Message: st.Message(),
	}

	switch st.Code() {
	case codes.PermissionDenied, codes.Unauthenticated:
		result.Status = audit.ResultStatusDenied
	default:
		result.Status = audit.ResultStatusError
	}

	return result
}
