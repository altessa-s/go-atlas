// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package health

import (
	"fmt"
	"testing"
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
			if got := tt.status.String(); got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestServingStatus_Values(t *testing.T) {
	if StatusUnknown != 0 {
		t.Error("StatusUnknown should be 0")
	}
	if StatusServing != 1 {
		t.Error("StatusServing should be 1")
	}
	if StatusNotServing != 2 {
		t.Error("StatusNotServing should be 2")
	}
	if StatusServiceUnknown != 3 {
		t.Error("StatusServiceUnknown should be 3")
	}
	if StatusDegraded != 4 {
		t.Error("StatusDegraded should be 4")
	}
}
