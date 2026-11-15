// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package auth

import (
	"context"

	"github.com/altessa-s/go-atlas/audit/decision"
)

// DecisionRecorder records a gRPC authentication decision.
type DecisionRecorder interface {
	Record(context.Context, decision.Decision) error
}
