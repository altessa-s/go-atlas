// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package health

import "fmt"

// ServingStatus represents a service's health status.
// The zero value is [StatusUnknown].
type ServingStatus int32

const (
	// StatusUnknown indicates health status is unknown.
	StatusUnknown ServingStatus = 0

	// StatusServing indicates the service is healthy.
	StatusServing ServingStatus = 1

	// StatusNotServing indicates the service is unhealthy.
	StatusNotServing ServingStatus = 2

	// StatusServiceUnknown indicates the service is not registered.
	StatusServiceUnknown ServingStatus = 3

	// StatusDegraded indicates the service is serving but with issues.
	StatusDegraded ServingStatus = 4
)

// String returns the UPPER_SNAKE_CASE representation of the status
// (e.g. "SERVING", "NOT_SERVING"). Unrecognized values produce "ServingStatus(<n>)".
func (s ServingStatus) String() string {
	switch s {
	case StatusUnknown:
		return "UNKNOWN"
	case StatusServing:
		return "SERVING"
	case StatusNotServing:
		return "NOT_SERVING"
	case StatusServiceUnknown:
		return "SERVICE_UNKNOWN"
	case StatusDegraded:
		return "DEGRADED"
	default:
		return fmt.Sprintf("ServingStatus(%d)", s)
	}
}
