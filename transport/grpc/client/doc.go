// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package client provides a generic gRPC client with connection management,
// retry, health checking, and error handling.
//
// It is designed to be a foundation for building service-specific gRPC clients
// with consistent behavior. The package provides:
//
//   - Connection management: Single connection or connection pool modes
//   - Retry with exponential backoff: Configurable retry policy using gRPC service config
//   - Health checking: Connection state monitoring
//   - Timeout management: Separate timeouts for query and mutation operations
//   - Error handling: Rich error type with field-level validation errors
//   - Authentication: Via WithDialOptions for custom credentials
//   - Logging: Automatic gRPC call logging via interceptors
//   - Proxy: declarative outbound proxy via WithProxy, WithProxyURL, or
//     WithoutProxy; defaults to grpc-go's HTTPS_PROXY env lookup
//
// # Basic Usage
//
// Create a client with minimal configuration:
//
//	c, err := client.New(ctx, "localhost:8080",
//		client.WithInsecure(),
//	)
//	if err != nil {
//		log.Fatal(err)
//	}
//	defer c.Close(ctx)
//
//	// Get a connection for making RPC calls
//	conn, err := c.GetConnection(ctx)
//	if err != nil {
//		return err
//	}
//	defer c.ReturnConnection(conn)
//
//	// Use the connection with generated gRPC client
//	serviceClient := pb.NewMyServiceClient(conn)
//
// # Production Configuration
//
// For production use, enable logging and retry:
//
//	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
//	c, err := client.New(ctx, "service.example.com:443",
//		client.WithLogger(logger),
//		client.WithRetry(),
//		client.WithDialOptions(grpc.WithPerRPCCredentials(creds)),
//		client.WithMutationTimeout(60*time.Second),
//		client.WithQueryTimeout(10*time.Second),
//	)
//
// # Connection Pooling
//
// For high-throughput scenarios, use connection pooling:
//
//	p := pool.New(
//		pool.WithPoolSize(20),
//		pool.WithMaxIdleTime(time.Hour),
//		pool.WithLogger(logger),
//	)
//	stop, _ := p.Start(ctx)
//	defer stop()
//
//	c, err := client.New(ctx, "service.example.com:443",
//		client.WithConnectionPool(p),
//		client.WithRetry(),
//		client.WithLogger(logger),
//	)
//
// # Custom Retry Configuration
//
// Configure custom retry behavior:
//
//	retryConfig := &client.RetryConfig{
//		MaxAttempts:          5,
//		InitialBackoff:       50 * time.Millisecond,
//		MaxBackoff:           5 * time.Second,
//		BackoffMultiplier:    1.5,
//		RetryableStatusCodes: []string{"UNAVAILABLE", "INTERNAL"},
//	}
//
//	c, err := client.New(ctx, "localhost:8080",
//		client.WithRetryConfig(retryConfig),
//		client.WithInsecure(),
//	)
//
// # Error Handling
//
// The package provides rich error handling with ClientError:
//
//	resp, err := serviceClient.GetUser(ctx, req)
//	if err != nil {
//		if ce := client.AsClientError(err); ce != nil {
//			switch {
//			case ce.IsNotFound():
//				// Handle not found
//			case ce.IsValidationError():
//				// Handle validation errors
//				for _, field := range ce.Fields {
//					log.Printf("Field %s: %s", field.Field, field.Message)
//				}
//			case ce.IsPermissionDenied():
//				// Handle permission denied
//			default:
//				// Handle other errors
//			}
//		}
//		return err
//	}
//
// # Custom Error Converter
//
// For service-specific error handling, provide a custom error converter:
//
//	converter := func(ctx context.Context, st *status.Status) error {
//		ce := client.ParseStatusError(st)
//		return &MyServiceError{
//			ClientError: ce,
//			CustomField: extractCustomField(st),
//		}
//	}
//
//	c, err := client.New(ctx, "localhost:8080",
//		client.WithErrorConverter(converter),
//	)
//
// # Building Service-Specific Clients
//
// This package is designed to be embedded in service-specific client packages:
//
//	package myservice
//
//	import "github.com/altessa-s/go-atlas/transport/grpc/client"
//
//	type Client struct {
//		*client.Client
//		usersClient *UsersClient
//	}
//
//	func New(ctx context.Context, address string, opts ...client.Option) (*Client, error) {
//		c, err := client.New(ctx, address, opts...)
//		if err != nil {
//			return nil, err
//		}
//		return &Client{Client: c}, nil
//	}
//
//	func (c *Client) Users() *UsersClient {
//		// Return service-specific client
//	}
package client
