// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package schedulerconfig_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-horai/schedule"

	schedulerconfig "github.com/altessa-s/go-atlas/config/scheduler"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// cronSpec is a named string type, as schemas often declare one.
type cronSpec string

func TestScheduleRule(t *testing.T) {
	t.Parallel()

	valid, invalid := "@every 5m", "*/5 * * * *"
	tests := []struct {
		name    string
		value   any
		wantErr bool
	}{
		{"empty string is skipped", "", false},
		{"nil pointer is skipped", (*string)(nil), false},
		{"descriptor", valid, false},
		{"six fields", "0 */5 * * * *", false},
		{"time zone prefix", "CRON_TZ=Europe/Moscow 0 0 3 * * *", false},
		{"pointer to valid", &valid, false},
		{"named string type", cronSpec("@hourly"), false},
		{"five fields", invalid, true},
		{"pointer to invalid", &invalid, true},
		{"garbage", "every day", true},
		{"named string type invalid", cronSpec("@sometimes"), true},
		{"time zone without schedule", "CRON_TZ=UTC", true}, // review finding R21-1: panicked in robfig/cron
		{"tz without schedule", "TZ=UTC", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := validation.Validate(tt.value, schedulerconfig.ScheduleRule)
			if !tt.wantErr {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, schedule.ErrInvalid)
		})
	}
}

func TestScheduleRule_NonString(t *testing.T) {
	t.Parallel()

	err := validation.Validate(42, schedulerconfig.ScheduleRule)
	require.Error(t, err)
	require.NotErrorIs(t, err, schedule.ErrInvalid)
}

// jobSection shows the rule inside another schema's Validate.
type jobSection struct {
	Schedule string
}

func (j *jobSection) Validate() error {
	return validation.ValidateStruct(j,
		validation.Field(&j.Schedule, validation.Required, schedulerconfig.ScheduleRule),
	)
}

func TestScheduleRule_InStruct(t *testing.T) {
	t.Parallel()

	require.NoError(t, (&jobSection{Schedule: "@daily"}).Validate())

	verrs := concurrencyFieldErrors(t, (&jobSection{Schedule: "0 0 * * *"}).Validate())
	require.ErrorIs(t, verrs["Schedule"], schedule.ErrInvalid)

	verrs = concurrencyFieldErrors(t, (&jobSection{}).Validate())
	blank, ok := errors.AsType[validation.Error](verrs["Schedule"])
	require.True(t, ok, "want validation.Error, got %v", verrs["Schedule"])
	require.Equal(t, validation.ErrRequired.Code(), blank.Code())
}
