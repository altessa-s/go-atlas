// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package postgresconfig

import (
	"cmp"
	"errors"
	"fmt"
	"math"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/altessa-s/go-atlas-core/types/redacted"

	tlsconfig "github.com/altessa-s/go-atlas/config/tls"
	validationconfig "github.com/altessa-s/go-nomos/validation"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// Config describes the primary connection pool and any reserved pools.
type Config struct {
	// DSN accepts a PostgreSQL URL or a libpq keyword/value connection string.
	DSN redacted.RedactedString `yaml:"dsn"`
	// TLS configures verified TLS for the primary pool and is inherited by reserved pools.
	TLS *tlsconfig.Client `yaml:"tls" default:"-"`
	// Pooling selects direct or transaction pooling.
	Pooling string `yaml:"pooling" default:"direct"`
	// MaxOpenConnections caps the primary pool.
	MaxOpenConnections int `yaml:"maxOpenConnections" default:"64"`
	// MaxIdleConnections caps idle connections in the primary pool.
	MaxIdleConnections int `yaml:"maxIdleConnections" default:"64"`
	// Pools allocates additional independent pools by name.
	Pools map[string]Pool `yaml:"pools"`
	// ConnectionMaxLifetime bounds connection age.
	ConnectionMaxLifetime time.Duration `yaml:"connectionMaxLifetime" default:"30m"`
	// ConnectionMaxIdleTime bounds time idle in the pool.
	ConnectionMaxIdleTime time.Duration `yaml:"connectionMaxIdleTime" default:"5m"`
	// ConnectTimeout bounds connection establishment.
	ConnectTimeout time.Duration `yaml:"connectTimeout" default:"5s"`
	// StatementTimeout bounds statement execution.
	StatementTimeout time.Duration `yaml:"statementTimeout" default:"10s"`
	// LockTimeout bounds lock acquisition.
	LockTimeout time.Duration `yaml:"lockTimeout" default:"3s"`
	// IdleTransactionTimeout bounds idle time inside a transaction.
	IdleTransactionTimeout time.Duration `yaml:"idleTransactionTimeout" default:"30s"`
	// MigrationTimeout bounds the optional startup migration hook.
	MigrationTimeout time.Duration `yaml:"migrationTimeout" default:"30s"`
	// ListingTimeout is the application budget for listing queries.
	ListingTimeout time.Duration `yaml:"listingTimeout" default:"3s"`
	// CountBudget is the application budget for exact counts. Zero disables probing.
	CountBudget time.Duration `yaml:"countBudget" default:"50ms"`
}

// Pool reserves connections for a workload. Zero timeouts, an empty DSN and an
// empty pooling mode inherit the primary pool's settings.
type Pool struct {
	// DSN optionally points to a separate PostgreSQL or PgBouncer entry.
	DSN redacted.RedactedString `yaml:"dsn"`
	// TLS replaces the entire primary TLS block for this endpoint.
	TLS *tlsconfig.Client `yaml:"tls" default:"-"`
	// Pooling overrides the primary pool's mode.
	Pooling string `yaml:"pooling"`
	// MaxOpenConnections is the required reservation size.
	MaxOpenConnections int `yaml:"maxOpenConnections"`
	// MaxIdleConnections defaults to the smaller of the reservation and primary idle limit.
	MaxIdleConnections *int `yaml:"maxIdleConnections"`
	// LockTimeout overrides the lock acquisition timeout.
	LockTimeout time.Duration `yaml:"lockTimeout"`
	// StatementTimeout overrides the statement timeout.
	StatementTimeout time.Duration `yaml:"statementTimeout"`
}

const (
	direct                 = "direct"
	transaction            = "transaction"
	defaultConnections     = 64
	defaultLifetime        = 30 * time.Minute
	defaultIdle            = 5 * time.Minute
	defaultConnect         = 5 * time.Second
	defaultStatement       = 10 * time.Second
	defaultLock            = 3 * time.Second
	defaultIdleTransaction = 30 * time.Second
	defaultMigration       = 30 * time.Second
	minimumListing         = 100 * time.Millisecond
	defaultListing         = 3 * time.Second
	defaultCount           = 50 * time.Millisecond
)

// Default supplies bounded defaults for a primary pool. Supply DSN before building.
func Default() Config {
	return Config{
		Pooling:                direct,
		MaxOpenConnections:     defaultConnections,
		MaxIdleConnections:     defaultConnections,
		ConnectionMaxLifetime:  defaultLifetime,
		ConnectionMaxIdleTime:  defaultIdle,
		ConnectTimeout:         defaultConnect,
		StatementTimeout:       defaultStatement,
		LockTimeout:            defaultLock,
		IdleTransactionTimeout: defaultIdleTransaction,
		MigrationTimeout:       defaultMigration,
		ListingTimeout:         defaultListing,
		CountBudget:            defaultCount,
	}
}

// Default adjusts the loader's idle default for a smaller explicit primary pool.
func (c *Config) Default() {
	if c.MaxOpenConnections > 0 {
		c.MaxIdleConnections = min(defaultConnections, c.MaxOpenConnections)
	}
}

// Validate checks pool bounds, timeout units, names and connection string shape.
// The runtime connector parses driver-specific connection parameters before opening a socket.
func (c *Config) Validate() error {
	if !validDSN(c.DSN) {
		return errors.New("invalid PostgreSQL DSN")
	}
	if err := validateTLS(c.DSN, c.TLS); err != nil {
		return err
	}
	const maxServerTimeout = math.MaxInt32 * time.Millisecond
	if err := validationconfig.ValidateStruct(c,
		validation.Field(&c.Pooling, validation.Required, validation.In(direct, transaction)),
		validation.Field(&c.MaxOpenConnections, validation.Required, validation.Min(1)),
		validation.Field(&c.MaxIdleConnections, validation.Min(0), validation.Max(c.MaxOpenConnections)),
		validation.Field(&c.ConnectionMaxLifetime, validation.Required, validation.Min(time.Second)),
		validation.Field(&c.ConnectionMaxIdleTime, validation.Required, validation.Min(time.Second)),
		validation.Field(&c.ConnectTimeout, validation.Required, validation.Min(time.Millisecond)),
		validation.Field(&c.StatementTimeout, validation.Required, validation.Min(time.Millisecond), validation.Max(maxServerTimeout)),
		validation.Field(&c.LockTimeout, validation.Required, validation.Min(time.Millisecond), validation.Max(maxServerTimeout)),
		validation.Field(&c.IdleTransactionTimeout, validation.Required, validation.Min(time.Millisecond), validation.Max(maxServerTimeout)),
		validation.Field(&c.MigrationTimeout, validation.Required, validation.Min(time.Millisecond)),
		validation.Field(&c.ListingTimeout, validation.Required, validation.Min(minimumListing)),
		validation.Field(&c.CountBudget, validationconfig.NonNegativeDuration()),
	); err != nil {
		return err
	}
	for name, p := range c.Pools {
		if name == "" || name == "foreground" || strings.TrimSpace(name) != name {
			return fmt.Errorf("invalid reserved pool name %q", name)
		}
		if err := validateTLS(cmp.Or(p.DSN, c.DSN), cmp.Or(p.TLS, c.TLS)); err != nil {
			return fmt.Errorf("pool %q: %w", name, err)
		}
		if err := p.validate(); err != nil {
			return fmt.Errorf("pool %q: %w", name, err)
		}
	}
	return nil
}

func (p Pool) validate() error {
	if p.MaxOpenConnections < 1 || p.MaxIdleConnections != nil && (*p.MaxIdleConnections < 0 || *p.MaxIdleConnections > p.MaxOpenConnections) {
		return errors.New("invalid connection limits")
	}
	if (!p.DSN.IsEmpty() && !validDSN(p.DSN)) || (p.Pooling != "" && p.Pooling != transaction && p.Pooling != direct) ||
		(p.LockTimeout != 0 && !timeout(p.LockTimeout)) || (p.StatementTimeout != 0 && !timeout(p.StatementTimeout)) {
		return errors.New("invalid connection settings")
	}
	return nil
}

func timeout(d time.Duration) bool { return d.Milliseconds() >= 1 && d.Milliseconds() <= math.MaxInt32 }

func validDSN(d redacted.RedactedString) bool {
	s := strings.TrimSpace(d.Expose())
	if strings.HasPrefix(s, "postgres://") || strings.HasPrefix(s, "postgresql://") {
		_, err := url.Parse(s)
		return err == nil
	}
	key, _, ok := strings.Cut(s, "=")
	return ok && key != "" && !strings.ContainsAny(strings.TrimSpace(key), " \t\n:/")
}

// Match one libpq assignment at a time so quoted passwords and escaped spaces
// cannot be mistaken for connection settings. Never include a token in an error.
const (
	libpqWhitespace = " \t\n\r\v\f"
	keyword         = `^([^[:space:]=]+)[[:space:]]*=[[:space:]]*`
	quotedValue     = `'((?:[^'\\]|\\.)*)'`
	// A single quote starts a quoted value only at its first byte.
	bareValue = `((?:(?:[^[:space:]\\']|\\.)(?:[^[:space:]\\]|\\.)*)?)`
)

var keywordValue = regexp.MustCompile("(?s)" + keyword + "(?:" + quotedValue + "|" + bareValue + ")")

func validateTLS(dsn redacted.RedactedString, cfg *tlsconfig.Client) error {
	if cfg == nil {
		return nil
	}
	if err := cfg.Validate(); err != nil {
		return errors.New("invalid PostgreSQL TLS certificate settings")
	}
	if !tlsCompatibleDSN(strings.TrimLeft(dsn.Expose(), libpqWhitespace)) {
		return errors.New("TLS requires a valid PostgreSQL DSN without sslmode=disable, allow or prefer")
	}
	return nil
}

func weakSSLMode(mode string) bool {
	switch mode {
	case "disable", "allow", "prefer":
		return true
	}
	return false
}

func tlsCompatibleDSN(s string) bool {
	if strings.HasPrefix(s, "postgres://") || strings.HasPrefix(s, "postgresql://") {
		u, err := url.Parse(s)
		if err != nil {
			return false
		}
		q, err := url.ParseQuery(u.RawQuery)
		if err != nil {
			return false
		}
		return !slices.ContainsFunc(q["sslmode"], weakSSLMode)
	}
	for s != "" {
		field := keywordValue.FindStringSubmatch(s)
		if field == nil || field[1] == "sslmode" && weakSSLMode(field[2]+field[3]) {
			return false
		}
		s = strings.TrimLeft(s[len(field[0]):], libpqWhitespace)
	}
	return true
}
