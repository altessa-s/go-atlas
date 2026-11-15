// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package recovery

//go:generate go tool optgen generate --type=advisoryListenerOptions --option-type=AdvisoryListenerOption --option-prefix=AdvisoryListener --output=advisory_options_gen.go

import "log/slog"

// advisoryListenerOptions configures [AdvisoryListener]. All fields are populated by generated [AdvisoryListenerOption]
// setters in advisory_options_gen.go.
type advisoryListenerOptions struct {
	// logger receives advisory events and subscription errors.
	logger *slog.Logger
}
