// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package postgresconfig

import (
	"errors"
	"math"
	"time"

	"github.com/altessa-s/go-atlas-core/types/redacted"

	validationconfig "github.com/altessa-s/go-nomos/validation"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// Migrations configures schema migrations run over their own direct session, apart from the application pools.
type Migrations struct {
	// DSN points directly at PostgreSQL, not at a transaction pooler. Only a migration run requires it.
	DSN redacted.RedactedString `yaml:"dsn"`
	// LockTimeout bounds each lock wait inside a migration, so DDL on a busy table fails instead of queueing its traffic.
	LockTimeout time.Duration `yaml:"lockTimeout" default:"5s"`
}

const defaultMigrationLock = 5 * time.Second

// DefaultMigrations supplies the default lock timeout. Supply DSN before migrating.
func DefaultMigrations() Migrations {
	return Migrations{LockTimeout: defaultMigrationLock}
}

// Validate checks the DSN shape when it is set, its compatibility with the primary TLS block it inherits, and that
// LockTimeout fits PostgreSQL's range and the primary migrationTimeout.
func (m Migrations) Validate(primary Config) error {
	if !m.DSN.IsEmpty() {
		if !validDSN(m.DSN) {
			return errors.New("invalid PostgreSQL migration DSN")
		}
		if err := validateTLS(m.DSN, primary.TLS); err != nil {
			return err
		}
	}
	return validationconfig.ValidateStruct(&m, validation.Field(&m.LockTimeout,
		validation.Required, validation.Min(time.Millisecond), validation.Max(min(primary.MigrationTimeout, maxServerTimeout))))
}

// MigrationSession returns the settings of the single direct session a migration run uses: the primary settings,
// including TLS, with m.DSN, direct pooling, one connection and no reserved pools. Statement and lock timeouts follow
// MigrationTimeout, which bounds the advisory lock wait and the run; m.LockTimeout is meant for the lock waits of each migration.
func (c Config) MigrationSession(m Migrations) Config {
	c.DSN = m.DSN
	c.Pooling = direct
	c.Pools = nil
	c.MaxOpenConnections = 1
	c.MaxIdleConnections = 1
	c.StatementTimeout = min(c.MigrationTimeout, maxServerTimeout)
	c.LockTimeout = c.StatementTimeout
	return c
}

const maxServerTimeout = math.MaxInt32 * time.Millisecond
