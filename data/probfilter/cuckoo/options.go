// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package cuckoo

//go:generate go run github.com/altessa-s/go-atlas/tools/codegen/optgen generate

import (
	"log/slog"
)

// options contains Cuckoo filter configuration.
type options struct {
	logger *slog.Logger
}
