// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package testhelpers

import (
	"context"
	"sync"
	"time"

	"github.com/altessa-s/go-atlas/observability/health"
)

// --- Mock Health Checker ---

// MockHealthChecker is a concurrency-safe implementation of [health.Checker] for testing.
// Set Delay to simulate slow health checks; if the context is canceled during
// the delay, [health.StatusNotServing] is returned.
type MockHealthChecker struct {
	mu     sync.Mutex
	Status health.ServingStatus
	Delay  time.Duration
	Calls  int
}

// NewMockHealthChecker returns a [MockHealthChecker] initialized with the given status.
func NewMockHealthChecker(status health.ServingStatus) *MockHealthChecker {
	return &MockHealthChecker{Status: status}
}

// CheckHealth increments the Calls counter and returns the current Status.
// If Delay is positive it sleeps for that duration; if ctx is canceled during
// the sleep it returns [health.StatusNotServing] immediately.
func (m *MockHealthChecker) CheckHealth(ctx context.Context) health.ServingStatus {
	m.mu.Lock()
	m.Calls++
	status := m.Status
	delay := m.Delay
	m.mu.Unlock()

	if delay > 0 {
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return health.StatusNotServing
		}
	}
	return status
}

// SetStatus updates the serving status in a concurrency-safe manner.
func (m *MockHealthChecker) SetStatus(s health.ServingStatus) {
	m.mu.Lock()
	m.Status = s
	m.mu.Unlock()
}

// Ensure MockHealthChecker implements health.Checker.
var _ health.Checker = (*MockHealthChecker)(nil)
