// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package errs

import (
	"errors"
)

var (
	// ErrInstanceNotFound is returned by Store.Get and Store.Update when no
	// saga instance exists for the requested ID.
	ErrInstanceNotFound = errors.New("saga: instance not found")

	// ErrInstanceExists is returned by Store.Create when an instance with the
	// same ID already exists. Callers use it to make Start idempotent.
	ErrInstanceExists = errors.New("saga: instance already exists")

	// ErrVersionConflict is returned by Store.Update when the persisted
	// version no longer matches the version carried by the supplied instance,
	// i.e. another coordinator advanced it concurrently. It is a benign signal:
	// the losing caller should drop the instance and let the winner proceed.
	ErrVersionConflict = errors.New("saga: optimistic concurrency conflict")

	// ErrDefinitionNotFound is returned when an orchestrator is asked to resume
	// an instance whose persisted definition name does not match the
	// orchestrator's definition.
	ErrDefinitionNotFound = errors.New("saga: definition mismatch")

	// ErrAlreadyTerminal is returned by Resume when the instance is already in
	// a terminal status (Completed, Compensated, or Failed) and there is
	// nothing left to run.
	ErrAlreadyTerminal = errors.New("saga: instance already terminal")

	// ErrCompensationFailed wraps the underlying cause when a compensation
	// exhausts its retries. The instance is moved to the Failed status and the
	// dead-letter hook fires.
	ErrCompensationFailed = errors.New("saga: compensation failed")

	// ErrNoCompensation is reported by Definition.Build (and panics in
	// MustBuild) when WithCompensationPolicy(PolicyEnforce) is set and a
	// compensatable step lacks a compensation function.
	ErrNoCompensation = errors.New("saga: step without compensation")

	// ErrEmptyDefinition is reported by Definition.Build when no steps were
	// added to the builder.
	ErrEmptyDefinition = errors.New("saga: definition has no steps")

	// ErrEmptyID is returned by Orchestrator.Start and Resume when the instance
	// ID is empty.
	ErrEmptyID = errors.New("saga: empty instance id")
)
