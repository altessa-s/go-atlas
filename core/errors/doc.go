// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package errors provides general error classification and wrapping utilities.
// It simplifies error handling by providing consistent patterns for context error checking
// and operation-based error wrapping.
//
// All functions are stateless and thread-safe.
//
// # Context Errors
//
// The package provides utilities for checking standard context errors like context.Canceled
// and context.DeadlineExceeded to simplify graceful shutdown and timeout logic.
//
//	func worker(ctx context.Context) error {
//		if errors.IsContextCanceled(ctx.Err()) {
//			return nil // graceful shutdown
//		}
//		return nil
//	}
//
// # Error Wrapping
//
// Consistent wrapping helpers reduce code duplication and ensure uniform error messages
// throughout the application.
//
//	func processData(data []byte) error {
//		if err := validate(data); err != nil {
//			return errors.WrapOperation(err, "validate data")
//		}
//		// Error format: "failed to validate data: <original error>"
//
//		if err := save(data); err != nil {
//			return errors.WrapField(err, "data")
//		}
//		// Error format: "field 'data': <original error>"
//
//		if err := publish(data); err != nil {
//			return errors.WrapOperationWithContext(err, "publish message", "topic 'events'")
//		}
//		// Error format: "failed to publish message on topic 'events': <original error>"
//		return nil
//	}
package errors
