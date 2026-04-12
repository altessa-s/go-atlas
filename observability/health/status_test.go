// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package health

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestServingStatus_String(t *testing.T) {
	tests := []struct {
		status ServingStatus
		want   string
	}{
		{StatusUnknown, "UNKNOWN"},
		{StatusServing, "SERVING"},
		{StatusNotServing, "NOT_SERVING"},
		{StatusServiceUnknown, "SERVICE_UNKNOWN"},
		{StatusDegraded, "DEGRADED"},
		{ServingStatus(99), fmt.Sprintf("ServingStatus(%d)", 99)},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			require.Equal(t, tt.want, tt.status.String())
		})
	}
}

func TestServingStatus_Values(t *testing.T) {
	require.Equal(t, ServingStatus(0), StatusUnknown)
	require.Equal(t, ServingStatus(1), StatusServing)
	require.Equal(t, ServingStatus(2), StatusNotServing)
	require.Equal(t, ServingStatus(3), StatusServiceUnknown)
	require.Equal(t, ServingStatus(4), StatusDegraded)
}
