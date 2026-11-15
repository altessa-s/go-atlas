// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mtls

import (
	"context"

	"github.com/altessa-s/go-atlas/audit/decision"
)

// DecisionRecorder records an authentication decision.
type DecisionRecorder interface {
	Record(context.Context, decision.Decision) error
}
