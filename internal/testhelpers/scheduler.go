// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package testhelpers

import (
	"context"
	"sync"

	schedtask "github.com/altessa-s/go-horai/task"
)

// MockTaskRegistrar is a concurrency-safe [schedtask.Registrar] that
// records the tasks registered with it instead of running them.
//
// It lets a test assert what a component asked the scheduler to do — the task
// ID, the schedule it derived from configuration — without standing up a real
// scheduler and waiting for a tick.
//
// Set Err to make Register fail, exercising the caller's error path.
type MockTaskRegistrar struct {
	// Err is returned by every Register call when non-nil. The task is still
	// recorded, so a test can assert both the attempt and the failure.
	Err error

	mu    sync.Mutex
	tasks []schedtask.Config
}

// Register records cfg and returns [MockTaskRegistrar.Err].
func (m *MockTaskRegistrar) Register(_ context.Context, cfg schedtask.Config) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.tasks = append(m.tasks, cfg)

	return m.Err
}

// Tasks returns a copy of the registered task configurations, in registration
// order.
func (m *MockTaskRegistrar) Tasks() []schedtask.Config {
	m.mu.Lock()
	defer m.mu.Unlock()

	return append([]schedtask.Config(nil), m.tasks...)
}

// Count returns the number of Register calls.
func (m *MockTaskRegistrar) Count() int {
	m.mu.Lock()
	defer m.mu.Unlock()

	return len(m.tasks)
}

// Task returns the registered task with the given ID and whether it was found.
func (m *MockTaskRegistrar) Task(id string) (schedtask.Config, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, task := range m.tasks {
		if task.ID == id {
			return task, true
		}
	}

	return schedtask.Config{}, false
}
