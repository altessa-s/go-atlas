// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package validators_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/config/internal/validators"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
)

func TestSQLNames(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name              string
		table, identifier bool
	}{
		{"", true, true},
		{"Events_1", true, true},
		{"_events", true, true},
		{"app.Events", true, false},
		{strings.Repeat("s", 63) + "." + strings.Repeat("t", 63), true, false},
		{strings.Repeat("t", 63), true, true},
		{strings.Repeat("t", 64), false, false},
		{strings.Repeat("s", 64) + ".events", false, false},
		{"1events", false, false},
		{"app..events", false, false},
		{"app.public.events", false, false},
		{"app.", false, false},
		{".events", false, false},
		{"events-outbox", false, false},
		{"events; DROP TABLE events", false, false},
		{`"events"`, false, false},
		{" events", false, false},
		{"événements", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			for _, check := range []struct {
				valid bool
				err   error
			}{
				{tc.table, validators.SQLTableName.Validate(tc.name)},
				{tc.identifier, validators.SQLIdentifier.Validate(tc.name)},
			} {
				if check.valid {
					require.NoError(t, check.err)
				} else {
					require.ErrorIs(t, check.err, coreerrs.ErrInvalidArgument)
				}
			}
		})
	}
}

func TestSQLNameValues(t *testing.T) {
	t.Parallel()
	type identifier string
	valid := "Events"
	for _, tc := range []struct {
		name  string
		value any
		valid bool
	}{
		{"nil", nil, true},
		{"nil pointer", (*string)(nil), true},
		{"string pointer", &valid, true},
		{"empty named string", identifier(""), true},
		{"named string", identifier("Events"), true},
		{"qualified named identifier", identifier("app.Events"), false},
		{"byte slice", []byte("Events"), true},
		{"qualified byte slice", []byte("app.Events"), false},
		{"wrong type", 42, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := validators.SQLIdentifier.Validate(tc.value)
			if tc.valid {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, coreerrs.ErrInvalidArgument)
			}
		})
	}
}
