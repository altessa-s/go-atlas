// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package protovalidator

import (
	"context"
	"testing"

	"github.com/altessa-s/go-atlas/transport/grpc/interceptors"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/metadata"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

var benchmarkResult error

func BenchmarkValidate_NoIgnore(b *testing.B) {
	validator := ValidatorFunc(func(ctx context.Context, msg proto.Message) error {
		return nil
	})

	ic := &interceptor{
		BaseInterceptor: interceptors.NewBaseInterceptor("protovalidator", nil),
		validator:       validator,
		opts:            defaultOptions(),
	}

	ctx := b.Context()
	ctx = metadata.NewContext(ctx, &metadata.CallMetadata{
		FullyMethodName: "/test.Service/Method",
	})

	d, ctx := ic.DrivenInterceptor(ctx)
	ri := d.(*requestInterceptor)

	req := &emptypb.Empty{}

	b.ResetTimer()
	for b.Loop() {
		benchmarkResult = ri.validate(ctx, req)
	}
}

func BenchmarkValidate_WithIgnore_NotInList(b *testing.B) {
	validator := ValidatorFunc(func(ctx context.Context, msg proto.Message) error {
		return nil
	})

	ic := &interceptor{
		BaseInterceptor: interceptors.NewBaseInterceptorWithFilter(
			"protovalidator",
			[]string{
				"/test.Service/IgnoredMethod1",
				"/test.Service/IgnoredMethod2",
				"/test.Service/IgnoredMethod3",
			},
			nil,
			nil,
		),
		validator: validator,
		opts:      defaultOptions(),
	}

	ctx := b.Context()
	ctx = metadata.NewContext(ctx, &metadata.CallMetadata{
		FullyMethodName: "/test.Service/AllowedMethod",
	})

	d, ctx := ic.DrivenInterceptor(ctx)
	ri := d.(*requestInterceptor)

	req := &emptypb.Empty{}

	b.ResetTimer()
	for b.Loop() {
		benchmarkResult = ri.validate(ctx, req)
	}
}

func BenchmarkValidate_WithIgnore_InList(b *testing.B) {
	validator := ValidatorFunc(func(ctx context.Context, msg proto.Message) error {
		return nil
	})

	ic := &interceptor{
		BaseInterceptor: interceptors.NewBaseInterceptorWithFilter(
			"protovalidator",
			[]string{
				"/test.service/method",
				"/test.service/ignoredmethod1",
				"/test.service/ignoredmethod2",
			},
			nil,
			nil,
		),
		validator: validator,
		opts:      defaultOptions(),
	}

	ctx := b.Context()
	ctx = metadata.NewContext(ctx, &metadata.CallMetadata{
		FullyMethodName: "/test.Service/Method",
	})

	d, ctx := ic.DrivenInterceptor(ctx)
	ri := d.(*requestInterceptor)

	req := &emptypb.Empty{}

	b.ResetTimer()
	for b.Loop() {
		benchmarkResult = ri.validate(ctx, req)
	}
}

func BenchmarkValidate_NonProtoMessage(b *testing.B) {
	validator := ValidatorFunc(func(ctx context.Context, msg proto.Message) error {
		return nil
	})

	ic := &interceptor{
		BaseInterceptor: interceptors.NewBaseInterceptor("protovalidator", nil),
		validator:       validator,
		opts:            defaultOptions(),
	}

	ctx := b.Context()
	d, ctx := ic.DrivenInterceptor(ctx)
	ri := d.(*requestInterceptor)

	req := "not a proto message"

	b.ResetTimer()
	for b.Loop() {
		benchmarkResult = ri.validate(ctx, req)
	}
}

func BenchmarkValidate_LargeIgnoreList(b *testing.B) {
	validator := ValidatorFunc(func(ctx context.Context, msg proto.Message) error {
		return nil
	})

	// Create large ignore list
	ignoreMethods := make([]string, 100)
	for i := range 100 {
		ignoreMethods[i] = "/test.service/ignoredmethod" + string(rune('a'+i%26)) + string(rune('0'+i/26))
	}

	ic := &interceptor{
		BaseInterceptor: interceptors.NewBaseInterceptorWithFilter(
			"protovalidator",
			ignoreMethods,
			nil,
			nil,
		),
		validator: validator,
		opts:      defaultOptions(),
	}

	ctx := b.Context()
	ctx = metadata.NewContext(ctx, &metadata.CallMetadata{
		FullyMethodName: "/test.Service/AllowedMethod",
	})

	d, ctx := ic.DrivenInterceptor(ctx)
	ri := d.(*requestInterceptor)

	req := &wrapperspb.StringValue{Value: "test"}

	b.ResetTimer()
	for b.Loop() {
		benchmarkResult = ri.validate(ctx, req)
	}
}

func BenchmarkPreCall(b *testing.B) {
	validator := ValidatorFunc(func(ctx context.Context, msg proto.Message) error {
		return nil
	})

	ic := &interceptor{
		BaseInterceptor: interceptors.NewBaseInterceptor("protovalidator", nil),
		validator:       validator,
		opts:            defaultOptions(),
	}

	ctx := b.Context()
	ctx = metadata.NewContext(ctx, &metadata.CallMetadata{
		FullyMethodName: "/test.Service/Method",
	})

	req := &emptypb.Empty{}

	b.ResetTimer()
	for b.Loop() {
		d, newCtx := ic.DrivenInterceptor(ctx)
		_, benchmarkResult = d.PreCall(newCtx, req)
	}
}

func BenchmarkPostMsgReceive(b *testing.B) {
	validator := ValidatorFunc(func(ctx context.Context, msg proto.Message) error {
		return nil
	})

	ic := &interceptor{
		BaseInterceptor: interceptors.NewBaseInterceptor("protovalidator", nil),
		validator:       validator,
		opts:            defaultOptions(),
	}

	ctx := b.Context()
	ctx = metadata.NewContext(ctx, &metadata.CallMetadata{
		FullyMethodName: "/test.Service/Method",
	})

	d, ctx := ic.DrivenInterceptor(ctx)
	ri := d.(*requestInterceptor)

	req := &emptypb.Empty{}

	b.ResetTimer()
	for b.Loop() {
		benchmarkResult = ri.PostMsgReceive(ctx, req, nil)
	}
}
