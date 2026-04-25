// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"errors"
	"fmt"
	"log/slog"

	"github.com/altessa-s/go-atlas/config"
	"github.com/altessa-s/go-atlas/data/audit"

	corefactory "github.com/altessa-s/go-atlas/core/factory"
)

// ErrDisabled is returned by [AuditorBuilder.Build] when auditing is disabled
// in the configuration. Callers should use [errors.Is] to check for this sentinel.
var ErrDisabled = errors.New("audit: disabled by configuration")

// AuditorBuilder assembles an [audit.Auditor] step by step using a fluent API.
// Create instances with [New]. Errors are accumulated and reported at [AuditorBuilder.Build] time.
// The builder is not safe for concurrent use.
type AuditorBuilder struct {
	corefactory.Base
	cfg  *config.Audit
	errs []error

	// Dependencies
	dispatcher audit.Dispatcher
}

// New creates an [AuditorBuilder] for the given audit config.
// Config can be nil — the error surfaces at [AuditorBuilder.Build] time.
func New(cfg *config.Audit) *AuditorBuilder {
	return &AuditorBuilder{
		Base: corefactory.NewBase(slog.New(slog.DiscardHandler)),
		cfg:  cfg,
	}
}

// Build assembles the audit Auditor. Errors from fluent methods are accumulated
// and reported here via [errors.Join].
//
// Returns (nil, nil) when the configuration has Enabled set to false.
func (b *AuditorBuilder) Build() (*audit.Auditor, error) {
	if err := corefactory.JoinErrors(b.errs); err != nil {
		return nil, err
	}

	if b.cfg == nil {
		return nil, fmt.Errorf("configuration is required")
	}

	if !b.cfg.Enabled {
		return nil, ErrDisabled
	}

	if b.dispatcher == nil {
		return nil, fmt.Errorf("dispatcher is required")
	}

	return b.createAuditor()
}

// createAuditor wraps the injected [audit.Dispatcher] with an Auditor and
// starts the facade. The dispatcher must already be started by the caller.
func (b *AuditorBuilder) createAuditor() (*audit.Auditor, error) {
	auditorOpts := []audit.Option{
		audit.WithLogger(b.Logger()),
	}

	auditor, err := audit.New(b.dispatcher, auditorOpts...)
	if err != nil {
		return nil, fmt.Errorf("create auditor: %w", err)
	}

	if err := auditor.Start(); err != nil {
		return nil, fmt.Errorf("start auditor: %w", err)
	}

	return auditor, nil
}
