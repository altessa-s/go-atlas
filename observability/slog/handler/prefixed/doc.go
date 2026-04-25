// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package prefixed provides a slog.Handler middleware that adds prefixes to log messages.
// Use it to categorize logs by component or subsystem.
//
// Example:
//
//	handler := prefixed.NewHandler(slog.NewTextHandler(os.Stdout, nil))
//	logger := slog.New(handler)
//	logger.Info("started", "prefix", "HTTP")
package prefixed
