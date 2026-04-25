// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package colorized provides a slog.Handler with color-coded terminal output.
// Use it for human-readable logs in CLI apps or development environments.
//
// Example:
//
//	handler := colorized.NewHandler(os.Stdout, colorized.WithLevel(slog.LevelInfo))
//	logger := slog.New(handler)
//	logger.Info("started", "port", 8080)
package colorized
