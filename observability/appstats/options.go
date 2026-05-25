// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package appstats

//go:generate go run github.com/altessa-s/go-atlas/cmd/optgen generate

import (
	"log/slog"
)

// options holds the internal configuration state for [StatsLogger].
// Modified through generated [Option] functions; never expose as a
// public struct.
type options struct {
	logger *slog.Logger `optgen:"notnil"`
}
