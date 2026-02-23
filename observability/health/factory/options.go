// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

//go:generate go run github.com/altessa-s/go-atlas/tools/codegen/optgen generate

import (
	"log/slog"
)

// options contains Factory configuration.
type options struct {
	// logger is the logger to be used by the coordinator.
	logger *slog.Logger `opt:"Logger"`
}
