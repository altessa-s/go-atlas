// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package protovalidator provides gRPC interceptors for validating incoming protocol buffer messages.
// It supports custom validation logic and integrates with the go-atlas interceptor chain.
//
// Example:
//
//	validator := protovalidator.ValidatorFunc(func(ctx context.Context, msg proto.Message) error {
//		if req, ok := msg.(*pb.CreateUserRequest); ok {
//			if req.Email == "" {
//				return status.Error(codes.InvalidArgument, "email is required")
//			}
//		}
//		return nil
//	})
//	interceptor := protovalidator.ServerInterceptor(validator,
//		protovalidator.WithLogger(logger),
//	)
//	server := grpc.NewServer(
//		grpc.UnaryInterceptor(interceptor.ServerUnaryInterceptor()),
//		grpc.StreamInterceptor(interceptor.ServerStreamInterceptor()),
//	)
package protovalidator
