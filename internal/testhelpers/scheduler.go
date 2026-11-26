// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package testhelpers

import (
	"context"
	"errors"
	"fmt"
	"sync"

	schedtask "github.com/altessa-s/go-horai/task"
)

// ErrTaskAlreadyRegistered is returned by [MockTaskRegistrar.Register] for a task
// ID that is already registered. It mirrors go-horai's ErrTaskAlreadyRegistered,
// which this package cannot import: generic test helpers depend only on core and
// the scheduler task contract.
var ErrTaskAlreadyRegistered = errors.New("task already registered")

// MockTaskRegistrar is a concurrency-safe [schedtask.Registrar] that
// records the tasks registered with it instead of running them.
//
// It lets a test assert what a component asked the scheduler to do — the task
// ID, the schedule it derived from configuration — without standing up a real
// scheduler and waiting for a tick.
//
// Like the real scheduler, it rejects a second registration of a task ID that
// was already registered successfully with [ErrTaskAlreadyRegistered]; a
// rejected duplicate is not recorded, and a failed registration does not
// reserve its ID, so the caller can retry it.
//
// Set Err, or ErrFor for one task, to make Register fail, exercising the
// caller's error path.
type MockTaskRegistrar struct {
	// Err is returned by every Register call when non-nil. The task is still
	// recorded, so a test can assert both the attempt and the failure.
	Err error
	// ErrFor, when non-nil, is called for each registration while Err is nil
	// and its result is returned. It runs with the registrar's lock held, so
	// it must not call the registrar's methods.
	ErrFor func(schedtask.Config) error

	mu         sync.Mutex
	tasks      []schedtask.Config
	registered map[string]struct{}
}

// Register records cfg and returns [MockTaskRegistrar.Err]. It returns an error
// wrapping [ErrTaskAlreadyRegistered] when cfg.ID was already registered.
func (m *MockTaskRegistrar) Register(_ context.Context, cfg schedtask.Config) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, dup := m.registered[cfg.ID]; dup {
		return fmt.Errorf("register task %q: %w", cfg.ID, ErrTaskAlreadyRegistered)
	}

	m.tasks = append(m.tasks, cfg)

	err := m.Err
	if err == nil && m.ErrFor != nil {
		err = m.ErrFor(cfg)
	}
	if err != nil {
		return err
	}

	if m.registered == nil {
		m.registered = make(map[string]struct{})
	}
	m.registered[cfg.ID] = struct{}{}

	return nil
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
