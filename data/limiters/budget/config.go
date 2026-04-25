// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package budget

import (
	"time"

	"github.com/altessa-s/go-atlas/config"

	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// Settings holds the validated runtime configuration for a [Limiter].
type Settings struct {
	// Limit is the maximum number of requests allowed within the period.
	Limit int64

	// Period is the time window for the budget counter.
	Period time.Duration
}

// Validate ensures limit is positive and period is at least one second.
func (s *Settings) Validate() error {
	return config.ValidateStruct(s,
		validation.Field(&s.Limit, validation.Required, validation.Min(int64(1))),
		validation.Field(&s.Period, validation.Required, validation.Min(time.Second)),
	)
}
