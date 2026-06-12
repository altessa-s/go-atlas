// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package driver provides the Driven Middleware pattern for HTTP middlewares.
//
// The Driven Middleware pattern separates lifecycle hooks from the HTTP handler
// mechanics, so middlewares that need pre/post processing are easier to implement.
// This pattern mirrors the gRPC interceptor driver pattern for consistency.
//
// # Basic Usage
//
// Implement the Driver interface with your pre/post request hooks:
//
//	type myDriver struct {
//	    startTime time.Time
//	}
//
//	func (d *myDriver) PreRequest(ctx context.Context, r *http.Request) (context.Context, error) {
//	    d.startTime = time.Now()
//	    return ctx, nil
//	}
//
//	func (d *myDriver) PostRequest(ctx context.Context, w ResponseInfo, r *http.Request, err error) {
//	    duration := time.Since(d.startTime)
//	    log.Printf("Request %s took %v", r.URL.Path, duration)
//	}
//
// Then implement DrivenMiddleware to create request-scoped drivers:
//
//	type myMiddleware struct{}
//
//	func (m *myMiddleware) DrivenMiddleware(ctx context.Context, r *http.Request) (Driver, context.Context) {
//	    return &myDriver{}, ctx
//	}
//
// Finally, wrap it with HTTPDrivenMiddleware to get a standard Middleware:
//
//	middleware := driver.HTTPDrivenMiddleware(&myMiddleware{})
package driver
