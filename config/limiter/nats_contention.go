// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package limiterconfig

import (
	"time"

	validationconfig "github.com/altessa-s/go-atlas/config/validation"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// NATSContention tunes how the NATS storage retries compare-and-swap
// conflicts on a hot key. Other storages ignore it. The defaults match the
// storage's own; a zero field also keeps the storage default.
type NATSContention struct {
	// MaxCASAttempts is the number of read-modify-write attempts per Allow
	// call before it fails with the storage's contention error.
	MaxCASAttempts int `yaml:"maxCASAttempts" default:"64"`

	// MaxCASBackoff caps the randomized pause after a revision conflict.
	MaxCASBackoff time.Duration `yaml:"maxCASBackoff" default:"20ms"`
}

// Validate rejects negative values.
func (c *NATSContention) Validate() error {
	return validationconfig.ValidateStruct(c,
		validation.Field(&c.MaxCASAttempts, validation.Min(0)),
		validation.Field(&c.MaxCASBackoff, validation.Min(time.Duration(0))),
	)
}
