// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package id

// Static is a [Provider] that returns a fixed, caller-supplied Service ID.
// It is safe for concurrent use because the value is immutable after
// construction. Typical uses include unit tests and scenarios where the
// Service ID is injected from external configuration.
type Static struct {
	id string // The fixed Service ID
}

// NewStatic creates a [Static] provider that always returns idVal. No
// validation is performed on the value; callers are responsible for ensuring
// it is meaningful (e.g. non-empty) for their use case.
func NewStatic(idVal string) *Static {
	return &Static{id: idVal}
}

// ID returns the fixed Service ID that was provided to [NewStatic].
func (s *Static) ID() string {
	return s.id
}

// Runtime check to ensure that the Static type implements the Provider interface.
var _ Provider = (*Static)(nil)
