// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package masking provides a slog.Handler that masks sensitive data in log records.
// Use it to prevent credentials and PII from appearing in logs.
//
// # Deferred values
//
// LogValuer representations are resolved before matching and traversing nested
// fields, including WithAttrs. Resolution and recursive traversal are bounded;
// exhausted nested values are replaced with a placeholder.
//
// # Usage
//
//	handler := masking.NewHandler(slog.NewJSONHandler(os.Stdout, nil), masking.WithDefaults())
//	logger := slog.New(handler)
//	logger.Info("login", "user", "john", "password", "secret123") // password masked
package masking
