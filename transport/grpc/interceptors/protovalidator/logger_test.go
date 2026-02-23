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
				t.Fatal("should not be called for ignored method")
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

			if tt.wantError && err == nil {
				t.Error("expected error but got nil")
			}
			if !tt.wantError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			// Check log output
			logOutput := buf.String()
			if logOutput == "" {
				t.Fatal("expected log output but got none")
			}

			var logEntry map[string]any
			if err := json.Unmarshal([]byte(logOutput), &logEntry); err != nil {
				t.Fatalf("failed to parse log output: %v", err)
			}

			// Check log level
			if level, ok := logEntry["level"].(string); ok {
				expectedLevel := strings.ToUpper(tt.wantLogLevel.String())
				if level != expectedLevel {
					t.Errorf("expected log level %s, got %s", expectedLevel, level)
				}
			}

			// Check log message
			if msg, ok := logEntry["msg"].(string); ok {
				if msg != tt.wantLogMessage {
					t.Errorf("expected log message %q, got %q", tt.wantLogMessage, msg)
				}
			}

			// Check message type is logged for validation success/failure
			if tt.req != nil && tt.wantLogMessage != "skipping validation for ignored method" {
				if msgType, ok := logEntry["message_type"].(string); ok {
					if msg, ok := tt.req.(proto.Message); ok {
						expectedType := string(msg.ProtoReflect().Descriptor().FullName())
						if msgType != expectedType {
							t.Errorf("expected message_type %q, got %q", expectedType, msgType)
						}
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

	if err == nil {
		t.Fatal("expected validation error")
	}

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
	if err != nil {
		t.Errorf("first validation should succeed: %v", err)
	}

	// Second call - failure
	err = ri.validate(ctx, &wrapperspb.StringValue{Value: "test"})
	if err == nil {
		t.Error("second validation should fail")
	}

	// Check we have two log entries
	logs := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(logs) != 2 {
		t.Errorf("expected 2 log entries, got %d", len(logs))
	}

	// Verify first is success, second is failure
	var firstLog, secondLog map[string]any
	if err := json.Unmarshal([]byte(logs[0]), &firstLog); err != nil {
		t.Fatalf("failed to parse first log: %v", err)
	}
	if err := json.Unmarshal([]byte(logs[1]), &secondLog); err != nil {
		t.Fatalf("failed to parse second log: %v", err)
	}

	if firstLog["msg"] != "validation successful" {
		t.Errorf("first log should be success, got %v", firstLog["msg"])
	}

	if secondLog["msg"] != "validation failed" {
		t.Errorf("second log should be failure, got %v", secondLog["msg"])
	}
}
