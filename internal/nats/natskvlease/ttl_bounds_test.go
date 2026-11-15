// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package natskvlease

import (
	"math"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestMigrationTTLRejectsOverflow(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		parse func(string) (time.Duration, error)
	}{
		{"source", parseMessageTTL}, {"copied", parseCopiedTTL},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			for _, raw := range []string{strconv.FormatInt(math.MaxInt64/int64(time.Second)+1, 10), "9223372036854775807", "-2"} {
				_, err := tc.parse(raw)
				require.Error(t, err, raw)
			}
			seconds := int64(math.MaxInt64 / time.Second)
			got, err := tc.parse(strconv.FormatInt(seconds, 10))
			require.NoError(t, err)
			require.Equal(t, time.Duration(seconds)*time.Second, got)
		})
	}
}
