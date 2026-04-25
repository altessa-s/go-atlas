// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package tlsfile

//go:generate go run github.com/altessa-s/go-atlas/tools/codegen/optgen generate --type=options  --all-fields

import (
	"context"
	"log/slog"

	"github.com/altessa-s/go-atlas/security/tlsutils"
)

// FileConfig holds configuration for the file provider.
type options struct {
	enableWatcher    bool
	logger           *slog.Logger
	reloadNotifyChan chan struct{}
	ocspStapler      tlsutils.OCSPStapler `optgen:"notnil"`
	ctx              context.Context      `opt:"Context"`
}
