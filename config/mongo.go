// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config

import (
	"errors"
	"time"

	"github.com/altessa-s/go-atlas/config/internal/validators"

	ozzo_rules "github.com/altessa-s/ozzo-rules"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// Default values for Mongodb configuration.
const (
	defaultMongoHost                 = "localhost:27017"
	defaultMongoMaxPoolSize          = uint64(100)
	defaultMongoMinPoolSize          = uint64(0)
	defaultMongoConnectTimeout       = 30 * time.Second
	defaultMongoMaxIdleTimeout       = time.Duration(0)
	defaultMongoZlibCompressionLevel = -1
	defaultMongoRetryReads           = true
	defaultMongoRetryWrites          = true
	defaultMongoDirectConnection     = false
)

// Mongodb represents the configuration for MongoDB database connections.
// It contains all necessary parameters for connecting to MongoDB including
// authentication, TLS, connection pooling, timeouts, and encryption settings.
//
// When ConnectionURI is set it takes priority over Hosts and Credentials.
// Fields not expressible through the URI (pool sizes, timeouts, TLS certificates,
// encryption) are applied on top. Database is always required.
//
// Example:
//
//	mongo := &config.Mongodb{
//		Database: "myapp",
//		Hosts:    []string{"mongo1:27017", "mongo2:27017"},
//	}
type Mongodb struct {
	// DirectConnection forces a direct connection to the specified host.
	// When true, bypasses automatic discovery of replica set topology.
	// Defaults to false.
	DirectConnection bool `yaml:"directConnection" default:"false"` //+

	// ConnectionURI is a full MongoDB connection string (e.g. "mongodb://user:pass@host:27017/db").
	// When set, it replaces Hosts, Credentials, ReplicaSet, DirectConnection, and Compressors.
	// Mutually exclusive with Credentials.
	ConnectionURI Secret `yaml:"connectionUri"`

	// Credentials contains optional authentication credentials.
	// If nil, the connection will attempt to connect without authentication.
	Credentials *MongodbCredentials `yaml:"credentials" default:"-"` //+

	// TLS contains optional TLS configuration for secure connections.
	// Required when using X.509 certificate authentication.
	TLS *TlsClient `yaml:"tls" default:"-"` //+

	// Database specifies the default database name.
	Database string `yaml:"database"`

	// ReplicaSet specifies the replica set name for replica set connections.
	// Leave empty for standalone server connections.
	ReplicaSet string `yaml:"replicaSet"` //+

	// Compressors specifies the list of compression algorithms to use.
	// Supported values: snappy, zlib, zstd.
	Compressors MongoCompressionTypes `yaml:"compressors"` //+

	// Hosts is the list of MongoDB server addresses.
	// Defaults to ["localhost:27017"].
	Hosts []string `yaml:"hosts" default:"localhost:27017"` //+

	// MaxPoolSize is the maximum number of connections in the connection pool.
	// Defaults to 100.
	MaxPoolSize uint64 `yaml:"maxPoolSize" default:"100"` //+

	// MinPoolSize is the minimum number of connections in the connection pool.
	// Defaults to 0.
	MinPoolSize uint64 `yaml:"minPoolSize" default:"0"` //+

	// ConnectTimeout is the maximum time to wait for a connection to be established.
	// Defaults to 30 seconds.
	ConnectTimeout time.Duration `yaml:"connectTimeout" default:"30s"` //+

	// MaxIdleTimeout is the maximum time a connection can be idle before being closed.
	// Set to 0 for no timeout (default).
	MaxIdleTimeout time.Duration `yaml:"maxIdleTimeout" default:"0"` //+

	// ZlibCompressionLevel specifies the compression level for zlib compression.
	// Valid range: -1 to 9, where -1 is default compression (default).
	ZlibCompressionLevel int `yaml:"zlibCompressionLevel" default:"-1"` //+

	// RetryReads enables automatic retrying of read operations.
	// Defaults to true.
	RetryReads bool `yaml:"retryReads" default:"true"` //+

	// RetryWrites enables automatic retrying of write operations.
	// Defaults to true.
	RetryWrites bool `yaml:"retryWrites" default:"true"` //+

	// Encryption contains optional client-side field level encryption settings.
	// If nil, no client-side encryption is used.
	Encryption *MongoEncryption `yaml:"encryption" default:"-"`
}

// MongoCompressionType represents the compression algorithm type for MongoDB connections.
// MongoDB supports multiple compression algorithms to reduce network traffic.
type MongoCompressionType string

const (
	// MongoCompressionTypeSnappy uses Snappy compression (fast, moderate ratio).
	MongoCompressionTypeSnappy MongoCompressionType = "snappy"

	// MongoCompressionTypeZlib uses zlib compression (configurable level via ZlibCompressionLevel).
	MongoCompressionTypeZlib MongoCompressionType = "zlib"

	// MongoCompressionTypeZstd uses Zstandard compression (high ratio, good speed).
	MongoCompressionTypeZstd MongoCompressionType = "zstd"
)

// String returns the string representation of the compression type.
func (ct MongoCompressionType) String() string {
	return string(ct)
}

// MongoCompressionTypes represents a slice of compression types.
// It provides utility methods for converting to string slices.
type MongoCompressionTypes []MongoCompressionType

// StringsSlice converts the compression types to a slice of strings.
// This is useful for passing to MongoDB driver functions.
func (cts MongoCompressionTypes) StringsSlice() []string {
	ss := make([]string, 0, len(cts))
	for _, ct := range cts {
		ss = append(ss, ct.String())
	}
	return ss
}

// UseConnectionURI reports whether ConnectionURI is set and should be used
// instead of Hosts and Credentials.
func (m *Mongodb) UseConnectionURI() bool {
	return !m.ConnectionURI.IsEmpty()
}

// DefaultMongodb returns a Mongodb configuration with default values.
// Note: Database is left as zero value since it is a required field.
func DefaultMongodb() Mongodb {
	return Mongodb{
		Hosts:                []string{defaultMongoHost},
		MaxPoolSize:          defaultMongoMaxPoolSize,
		MinPoolSize:          defaultMongoMinPoolSize,
		ConnectTimeout:       defaultMongoConnectTimeout,
		MaxIdleTimeout:       defaultMongoMaxIdleTimeout,
		ZlibCompressionLevel: defaultMongoZlibCompressionLevel,
		RetryReads:           defaultMongoRetryReads,
		RetryWrites:          defaultMongoRetryWrites,
		DirectConnection:     defaultMongoDirectConnection,
	}
}

// Validate performs comprehensive validation on the MongoDB configuration.
// It validates hosts, direct connection settings, timeouts, compression types,
// database name, and ensures TLS is configured when using X.509 authentication.
//
// When ConnectionURI is set, Hosts are not required and Credentials must not be set
// (conflict error). Database is always required.
//
// Returns an error if any validation rules fail.
func (m *Mongodb) Validate() error {
	if err := m.validateConnectionURIConflicts(); err != nil {
		return err
	}

	return ValidateStruct(m,
		validation.Field(&m.Hosts,
			validation.When(!m.UseConnectionURI(),
				validation.Required.Error("minimum one server must be specified"))),
		validation.Field(&m.DirectConnection,
			validation.When(!m.UseConnectionURI(), validators.MongoDirectionConnect(m.Hosts))),
		validation.Field(&m.ConnectTimeout, ozzo_rules.DurationOrZero()),
		validation.Field(&m.MaxIdleTimeout, ozzo_rules.DurationOrZero()),
		validation.Field(&m.Compressors, validation.Each(
			ozzo_rules.OneOf(
				MongoCompressionTypeSnappy,
				MongoCompressionTypeZlib,
				MongoCompressionTypeZstd))),
		validation.Field(&m.Database, validation.Required),

		// TLS is required if credentials are specified and auth mechanism is X509
		validation.Field(&m.TLS, validation.Required.When(m.Credentials != nil &&
			m.Credentials.AuthMechanism == MongoAuthMechanismTypeX509)),

		validation.Field(&m.Encryption, validation.NilOrNotEmpty),
	)
}

// validateConnectionURIConflicts returns an error if ConnectionURI is set
// together with Credentials.
func (m *Mongodb) validateConnectionURIConflicts() error {
	if !m.UseConnectionURI() {
		return nil
	}

	var errs []error
	if m.Credentials != nil {
		errs = append(errs, errors.New("credentials must not be set when connectionUri is used"))
	}

	return errors.Join(errs...)
}
