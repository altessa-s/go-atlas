// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package opa

import (
	"context"
)

// Result represents the result of a policy evaluation.
type Result struct {
	// DecisionID is a unique identifier for the evaluation.
	DecisionID string `json:"decision_id,omitempty"`
	// Allow indicates whether the request is permitted.
	Allow bool `json:"allow"`
}

// Evaluator defines the interface for checking permissions using OPA policies.
type Evaluator interface {
	// Evaluate evaluates the policy with the given input.
	Evaluate(ctx context.Context, input any) (*Result, error)
	// Query returns the Rego query used for evaluation.
	Query() string
}
