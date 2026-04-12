// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package protovalidator

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/transport/grpc/interceptors"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/metadata"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestValidate_WithLogger(t *testing.T) {
	tests := []struct {
		name           string
		validator      Validator
		req            any
		ignoreMethods  []string
		methodName     string
		wantLogLevel   slog.Level
		wantLogMessage string
		wantError      bool
	}{
		{
			name: "successful validation logs debug",
			validator: ValidatorFunc(func(_ context.Context, _ proto.Message) error {
				return nil
			}),
			req:            &emptypb.Empty{},
			methodName:     "/test.Service/Method",
			wantLogLevel:   slog.LevelDebug,
			wantLogMessage: "validation successful",
		},
		{
			name: "validation error logs warn",
			validator: ValidatorFunc(func(_ context.Context, _ proto.Message) error {
				return errors.New("validation error")
			}),
			req:            &emptypb.Empty{},
			methodName:     "/test.Service/Method",
			wantLogLevel:   slog.LevelWarn,
			wantLogMessage: "validation failed",
			wantError:      true,
		},
		{
			name: "ignored method logs debug",
			validator: ValidatorFunc(func(_ context.Context, _ proto.Message) error {
				require.Fail(t, "should not be called for ignored method")
				return nil
			}),
			req:            &emptypb.Empty{},
			ignoreMethods:  []string{"/test.service/ignoredmethod"},
			methodName:     "/test.Service/IgnoredMethod",
			wantLogLevel:   slog.LevelDebug,
			wantLogMessage: "skipping validation for ignored method",
		},
		{
			name: "panic logs error",
			validator: ValidatorFunc(func(_ context.Context, _ proto.Message) error {
				panic("test panic")
			}),
			req:            &emptypb.Empty{},
			methodName:     "/test.Service/Method",
			wantLogLevel:   slog.LevelError,
			wantLogMessage: "validator panic recovered",
			wantError:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{
				Level: slog.LevelDebug,
			}))

			ic := &interceptor{
				BaseInterceptor: interceptors.NewBaseInterceptorWithFilter(
					"protovalidator",
					tt.ignoreMethods,
					nil,
					logger,
				),
				validator: tt.validator,
				opts:      defaultOptions(),
			}

			ctx := t.Context()
			ctx = metadata.NewContext(ctx, &metadata.CallMetadata{
				FullyMethodName: tt.methodName,
			})

			d, ctx := ic.DrivenInterceptor(ctx)
			ri := d.(*requestInterceptor)

			err := ri.validate(ctx, tt.req)

			require.False(t, tt.wantError && err == nil)
			require.False(t, !tt.wantError && err != nil)

			// Check log output
			logOutput := buf.String()
			require.NotEqual(t, "", logOutput)

			var logEntry map[string]any
			err = json.Unmarshal([]byte(logOutput), &logEntry)
			require.NoError(t, err)

			// Check log level
			if level, ok := logEntry["level"].(string); ok {
				expectedLevel := strings.ToUpper(tt.wantLogLevel.String())
				require.Equal(t, expectedLevel, level)
			}

			// Check log message
			if msg, ok := logEntry["msg"].(string); ok {
				require.Equal(t, tt.wantLogMessage, msg)
			}

			// Check message type is logged for validation success/failure
			if tt.req != nil && tt.wantLogMessage != "skipping validation for ignored method" {
				if msgType, ok := logEntry["message_type"].(string); ok {
					if msg, ok := tt.req.(proto.Message); ok {
						expectedType := string(msg.ProtoReflect().Descriptor().FullName())
						require.Equal(t, expectedType, msgType)
					}
				}
			}
		})
	}
}

func TestValidate_WithoutLogger(t *testing.T) {
	// Test that operations work correctly when logger is nil (uses discard logger)
	validator := ValidatorFunc(func(_ context.Context, _ proto.Message) error {
		return errors.New("validation error")
	})

	ic := &interceptor{
		BaseInterceptor: interceptors.NewBaseInterceptor("protovalidator", nil),
		validator:       validator,
		opts:            defaultOptions(),
	}

	ctx := t.Context()
	d, ctx := ic.DrivenInterceptor(ctx)
	ri := d.(*requestInterceptor)

	err := ri.validate(ctx, &emptypb.Empty{})

	require.NotNil(t, err, "expected validation error")

	// Should not panic even without explicit logger
}

func TestValidate_LoggerWithMultipleMessages(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	callCount := 0
	validator := ValidatorFunc(func(_ context.Context, _ proto.Message) error {
		callCount++
		if callCount == 1 {
			return nil
		}
		return errors.New("second validation fails")
	})

	ic := &interceptor{
		BaseInterceptor: interceptors.NewBaseInterceptor("protovalidator", logger),
		validator:       validator,
		opts:            defaultOptions(),
	}

	ctx := t.Context()
	ctx = metadata.NewContext(ctx, &metadata.CallMetadata{
		FullyMethodName: "/test.Service/Method",
	})

	d, ctx := ic.DrivenInterceptor(ctx)
	ri := d.(*requestInterceptor)

	// First call - success
	err := ri.validate(ctx, &emptypb.Empty{})
	require.NoError(t, err)

	// Second call - failure
	err = ri.validate(ctx, &wrapperspb.StringValue{Value: "test"})
	require.NotNil(t, err, "second validation should fail")

	// Check we have two log entries
	logs := strings.Split(strings.TrimSpace(buf.String()), "\n")
	require.Len(t, logs, 2)

	// Verify first is success, second is failure
	var firstLog, secondLog map[string]any
	err = json.Unmarshal([]byte(logs[0]), &firstLog)
	require.NoError(t, err)
	err = json.Unmarshal([]byte(logs[1]), &secondLog)
	require.NoError(t, err)

	require.Equal(t, "validation successful", firstLog["msg"])

	require.Equal(t, "validation failed", secondLog["msg"])
}
