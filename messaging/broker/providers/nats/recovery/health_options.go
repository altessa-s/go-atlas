// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package recovery

//go:generate go tool optgen generate --type=healthMonitorOptions --option-type=HealthMonitorOption --option-prefix=HealthMonitor --output=health_options_gen.go

import "log/slog"

// healthMonitorOptions configures [HealthMonitor]. All fields are populated by generated [HealthMonitorOption] setters in
// health_options_gen.go.
type healthMonitorOptions struct {
	// logger receives health-check findings.
	logger *slog.Logger
}
