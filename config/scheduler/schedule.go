// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package schedulerconfig

import (
	"github.com/altessa-s/go-horai/schedule"

	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// ScheduleRule validates a cron schedule spec with the grammar of every
// go-atlas scheduler ([schedule.Validate]): six fields with
// leading seconds or a descriptor such as "@every 5m", optionally prefixed
// with "CRON_TZ=<zone> ". It accepts a string-kinded field or a pointer to
// one. Like the ozzo built-in rules it skips nil and empty values; pair it
// with validation.Required for a mandatory schedule, and check a disabling
// sentinel such as "off" before it. A failure matches
// [schedule.ErrInvalid].
//
//	validation.Field(&c.Schedule, validation.Required, schedulerconfig.ScheduleRule)
var ScheduleRule validation.Rule = validation.By(validateSchedule)

// validateSchedule is the [validation.RuleFunc] behind [ScheduleRule].
func validateSchedule(value any) error {
	value, isNil := validation.Indirect(value)
	if isNil || validation.IsEmpty(value) {
		return nil
	}
	spec, err := validation.EnsureString(value)
	if err != nil {
		return err
	}
	return schedule.Validate(spec)
}
