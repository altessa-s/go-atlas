// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package protovalidator

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/altessa-s/go-aip/fieldinfo"

	"github.com/altessa-s/go-atlas/internal/diagnostics"
	"github.com/altessa-s/go-atlas/internal/transport/updatevalidation"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/auth"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/driver"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/errstatus"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/recovery"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	sharedrecovery "github.com/altessa-s/go-atlas/internal/transport/recovery"
	sharedmetadata "github.com/altessa-s/go-atlas/transport/grpc/interceptors/metadata"
)

// Validator defines a validator for proto messages.
type Validator interface {
	Validate(context.Context, proto.Message) error
}

// ValidatorFunc adapts a function to the Validator interface.
type ValidatorFunc func(context.Context, proto.Message) error

// Validate implements the Validator interface by calling the underlying function.
func (f ValidatorFunc) Validate(ctx context.Context, msg proto.Message) error {
	return f(ctx, msg)
}

const interceptorName = "protovalidator"

// Name returns the interceptor name used for dependency resolution and chain ordering.
func Name() string { return interceptorName }

// ID is a lightweight [interceptors.Interceptor] reference for this package,
// suitable for passing to exclusion lists.
var ID = interceptors.Ref(interceptorName)

var (
	_ driver.DrivenInterceptor = (*interceptor)(nil)
	_ driver.DriverStream      = (*requestInterceptor)(nil)
	_ interceptors.Interceptor = (*interceptor)(nil)
)

// interceptor implements the protovalidator interceptor.
type interceptor struct {
	interceptors.BaseInterceptor
	validator Validator
	opts      *options
}

// Dependencies returns interceptors that protovalidator requires to run before it.
// Authentication rejects unauthenticated calls before validation. Recovery
// covers the interceptor.
func (i *interceptor) Dependencies() []string {
	return []string{sharedmetadata.Name(), auth.Name(), recovery.Name()}
}

// requestInterceptor handles a single request with its own state.
type requestInterceptor struct {
	*interceptor
	meta *sharedmetadata.CallMetadata
}

// DrivenInterceptor implements the driver.DrivenInterceptor interface.
func (i *interceptor) DrivenInterceptor(ctx context.Context) (driver.Driver, context.Context) {
	// Metadata is guaranteed to be in context by the chain
	meta, _ := sharedmetadata.FromContext(ctx)

	// Create a new instance for each request to avoid race conditions
	ri := &requestInterceptor{
		interceptor: i,
		meta:        meta,
	}
	return ri, ctx
}

// ServerInterceptor creates a new ServerInterceptor that validates incoming proto messages.
func ServerInterceptor(validator Validator, opt ...Option) interceptors.ServerInterceptor {
	if validator == nil {
		panic("protovalidator: validator cannot be nil")
	}
	opts := newOptions(opt...)

	ic := &interceptor{
		BaseInterceptor: interceptors.NewBaseInterceptorWithFilter(
			interceptorName,
			opts.ignoreMethods,
			opts.ignorePatterns,
			opts.logger,
		),
		validator: validator,
		opts:      opts,
	}

	return interceptors.ServerDrivenInterceptor(ic)
}

// PreCall is called before the RPC handler execution.
// For protovalidator, validation happens here for unary calls.
func (ri *requestInterceptor) PreCall(ctx context.Context, req any) (any, error) {
	if ri.meta != nil && ri.meta.IsClient {
		return nil, nil //nolint:nilnil // Client-side calls are not validated
	}

	if err := ri.validate(ctx, req); err != nil {
		return nil, err
	}
	return nil, nil //nolint:nilnil // Proceed to handler
}

// PostCall is called after the RPC handler execution.
// Protovalidator doesn't need to do anything post-call.
func (ri *requestInterceptor) PostCall(_ context.Context, _ any, err error) error {
	return err
}

// PostMsgReceive is called after a message is received in streaming RPCs.
// Validates each incoming message in the stream.
func (ri *requestInterceptor) PostMsgReceive(ctx context.Context, req any, err error) error {
	if err != nil {
		return err
	}

	if ri.meta != nil && ri.meta.IsClient {
		return nil
	}

	return ri.validate(ctx, req)
}

// PostMsgSent is called after a message is sent in streaming RPCs.
// Protovalidator doesn't validate outgoing messages.
func (ri *requestInterceptor) PostMsgSent(_ context.Context, _ any, err error) error {
	return err
}

// validate performs the actual validation of the proto message.
func (ri *requestInterceptor) validate(ctx context.Context, req any) (err error) {
	// Fast path: check if it's a proto message first
	msg, ok := req.(proto.Message)
	if !ok {
		return nil
	}

	// Check if method should be ignored
	method := ri.meta.Method()
	if method != "" && ri.ShouldIgnore(method) {
		ri.LogDebug(ctx, "skipping validation for ignored method", method)
		if sel := updatevalidation.FromContext(ctx, msg); sel != nil {
			return sel.MaskError
		}
		return nil
	}

	// Perform validation with panic recovery
	defer func() {
		if r := recover(); r != nil {
			attrs := []slog.Attr{slog.String("category", sharedrecovery.PanicCategory)}
			if ri.opts.logRawErrors {
				attrs = append(attrs,
					slog.String("panic", diagnostics.ErrorText(fmt.Sprint(r), diagnostics.DefaultMaxErrorLength)),
					slog.Any("stack_trace", sharedrecovery.StackTrace(1)),
				)
			}
			ri.LogError(ctx, "validator panic recovered", method, nil, attrs...)
			err = interceptors.NewError(status.New(codes.Internal, "Internal Error"), sharedrecovery.ErrPanicRecovered)
		}
	}()

	msgType := string(msg.ProtoReflect().Descriptor().FullName())

	if validationErr := ri.validator.Validate(ctx, msg); validationErr != nil {
		st, isStatus := status.FromError(validationErr)
		code := codes.InvalidArgument
		if isStatus {
			code = st.Code()
		}
		var logErr error
		if ri.opts.logRawErrors {
			logErr = validationErr
		}
		ri.LogWarn(ctx, "validation failed", method, diagnostics.LogError(logErr),
			slog.String("classification", diagnostics.ErrorClass(validationErr)),
			slog.String("message_type", msgType),
			slog.String("grpc.code", code.String()),
		)

		// Wrap non-gRPC errors. The validator's raw message may echo submitted
		// values or internal detail, so keep it internal (NewError preserves it for
		// logging) and hand the client only a generic status.
		if !isStatus {
			return interceptors.NewError(
				status.New(codes.InvalidArgument, "Validation Failed"),
				validationErr,
			)
		}
		return validationErr
	}

	if ri.opts.fieldInfoNormalization {
		if err := fieldinfo.Normalize(msg); err != nil {
			var logErr error
			if ri.opts.logRawErrors {
				logErr = err
			}
			ri.LogWarn(ctx, "field format normalization failed", method, diagnostics.LogError(logErr),
				slog.String("classification", diagnostics.ErrorClass(err)),
				slog.String("message_type", msgType),
			)
			if violation, ok := errors.AsType[*fieldinfo.ViolationError](err); ok && len(violation.Violations) > 0 {
				first := violation.Violations[0]
				return interceptors.NewError(errstatus.BadRequest(first.Path, first.Reason), err)
			}
			return interceptors.NewError(status.New(codes.InvalidArgument, "Validation Failed"), err)
		}
	}

	ri.LogDebug(ctx, "validation successful", method,
		slog.String("message_type", msgType),
	)

	return nil
}
