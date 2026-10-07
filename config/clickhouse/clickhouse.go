// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package clickhouseconfig

import (
	"errors"
	"time"

	"github.com/altessa-s/go-atlas/core/collections/slices"
	"github.com/altessa-s/go-atlas/core/types/redacted"

	tlsconfig "github.com/altessa-s/go-atlas/config/tls"
	validationconfig "github.com/altessa-s/go-atlas/config/validation"
	ozzo_rules "github.com/altessa-s/ozzo-rules"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// Default values for the ClickHouse configuration.
const (
	DefaultHost            = "localhost:9000"
	DefaultDatabase        = "default"
	DefaultDialTimeout     = 5 * time.Second
	DefaultReadTimeout     = 30 * time.Second
	DefaultMaxOpenConns    = 10
	DefaultMaxIdleConns    = 5
	DefaultConnMaxLifetime = time.Hour
)

// Compression selects the wire compression method.
type Compression string

// Supported compression methods.
const (
	CompressionNone    Compression = "none"
	CompressionLZ4     Compression = "lz4"
	CompressionZSTD    Compression = "zstd"
	CompressionGZIP    Compression = "gzip"
	CompressionDeflate Compression = "deflate"
	CompressionBrotli  Compression = "br"
)

// String returns the string representation of the compression method.
func (c Compression) String() string { return string(c) }

// AllCompressions returns every supported compression method.
func AllCompressions() []Compression {
	return []Compression{CompressionNone, CompressionLZ4, CompressionZSTD, CompressionGZIP, CompressionDeflate, CompressionBrotli}
}

// Config is the configuration of a ClickHouse connection.
//
// When ConnectionURI is set it replaces Hosts, Database, Username and
// Password; settings the DSN cannot express (TLS, pool sizing, timeouts,
// compression, query settings) are applied on top.
//
// Example:
//
//	cfg := clickhouseconfig.Config{
//		Hosts:    []string{"clickhouse-1:9000", "clickhouse-2:9000"},
//		Database: "analytics",
//		Username: "writer",
//	}
type Config struct {
	// TLS contains optional TLS configuration. When nil the connection is
	// made without TLS.
	TLS *tlsconfig.Client `yaml:"tls" default:"-"`

	// ConnectionURI is a full ClickHouse DSN, for example
	// "clickhouse://user:pass@host:9000/analytics". When set, it replaces
	// Hosts, Database, Username and Password.
	ConnectionURI redacted.RedactedString `yaml:"connectionURI"`

	// Hosts lists the servers to connect to, as host:port. The client fails
	// over between them.
	Hosts []string `yaml:"hosts" default:"localhost:9000"`

	// Database is the database holding the target tables.
	Database string `yaml:"database" default:"default"`

	// Username is the ClickHouse user. Empty uses the server's "default" user.
	Username string `yaml:"username"`

	// Password is the password for Username.
	Password redacted.RedactedString `yaml:"password"`

	// DialTimeout bounds establishing a connection.
	DialTimeout time.Duration `yaml:"dialTimeout" default:"5s"`

	// ReadTimeout bounds reading a query result.
	ReadTimeout time.Duration `yaml:"readTimeout" default:"30s"`

	// MaxOpenConns caps the number of open connections in the pool.
	MaxOpenConns int `yaml:"maxOpenConns" default:"10"`

	// MaxIdleConns caps the number of idle connections kept in the pool.
	MaxIdleConns int `yaml:"maxIdleConns" default:"5"`

	// ConnMaxLifetime is how long a pooled connection may be reused.
	ConnMaxLifetime time.Duration `yaml:"connMaxLifetime" default:"1h"`

	// Compression selects the wire compression method. LZ4 is the ClickHouse
	// default and costs little for a large bandwidth saving.
	Compression Compression `yaml:"compression" default:"lz4"`

	// Settings carries server-side query settings applied to every
	// statement, for example {"max_execution_time": "60"}.
	Settings map[string]string `yaml:"settings"`
}

// UseConnectionURI reports whether ConnectionURI is set and replaces Hosts,
// Database and the authentication fields.
func (c *Config) UseConnectionURI() bool {
	return !c.ConnectionURI.IsEmpty()
}

// Default returns a configuration with default values.
func Default() Config {
	return Config{
		Hosts:           []string{DefaultHost},
		Database:        DefaultDatabase,
		DialTimeout:     DefaultDialTimeout,
		ReadTimeout:     DefaultReadTimeout,
		MaxOpenConns:    DefaultMaxOpenConns,
		MaxIdleConns:    DefaultMaxIdleConns,
		ConnMaxLifetime: DefaultConnMaxLifetime,
		Compression:     CompressionLZ4,
	}
}

// Validate validates the configuration. With ConnectionURI set, Hosts and
// Database are not required and Username and Password must not be set.
func (c *Config) Validate() error {
	if err := c.validateConnectionURIConflicts(); err != nil {
		return err
	}

	return validationconfig.ValidateStruct(c,
		validation.Field(&c.Hosts,
			validation.When(!c.UseConnectionURI(),
				validation.Required.Error("minimum one host must be specified"))),
		validation.Field(&c.Database,
			validation.When(!c.UseConnectionURI(), validation.Required)),
		validation.Field(&c.TLS, validation.When(c.TLS != nil, validation.Required)),
		validation.Field(&c.DialTimeout, ozzo_rules.Duration()),
		validation.Field(&c.ReadTimeout, ozzo_rules.DurationOrZero()),
		// Required rejects zero: ozzo skips every other rule for an empty
		// value, so Min(1) alone would let 0 through.
		validation.Field(&c.MaxOpenConns,
			validation.Required.Error("must be greater than 0"),
			validation.Min(1).Error("must be greater than 0")),
		validation.Field(&c.MaxIdleConns, validation.Min(0).Error("must be greater or equal 0")),
		validation.Field(&c.ConnMaxLifetime, ozzo_rules.DurationOrZero()),
		validation.Field(&c.Compression, ozzo_rules.OneOf(AllCompressions()...)),
	)
}

// validateConnectionURIConflicts rejects fields the DSN already carries.
func (c *Config) validateConnectionURIConflicts() error {
	if !c.UseConnectionURI() {
		return nil
	}

	var errs []error
	errs = slices.AppendIf(errs, c.Username != "",
		errors.New("username must not be set when connectionURI is used"))
	errs = slices.AppendIf(errs, !c.Password.IsEmpty(),
		errors.New("password must not be set when connectionURI is used"))

	return errors.Join(errs...)
}
