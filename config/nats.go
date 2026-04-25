// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config

import (
	"errors"
	"time"

	ozzo_rules "github.com/altessa-s/ozzo-rules"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// Default values for Nats configuration.
const (
	defaultNatsHost           = "nats://localhost:4222"
	defaultNatsConnectTimeout = 5 * time.Second
	defaultNatsReconnectWait  = 10 * time.Second
	defaultNatsPingInterval   = 10 * time.Second
	defaultNatsMaxReconnect   = 0
	defaultNatsMaxPingsOut    = 3
)

// Nats represents the configuration for NATS messaging system connections.
// It contains all necessary parameters for connecting to NATS servers including
// authentication, TLS settings, connection timeouts, and reconnection behavior.
//
// When ConnectionURI is set it takes priority over Hosts and authentication fields
// (Username, Password, Token, NkeySeed). Fields not expressible through the URI
// (TLS certificates, timeouts, consumers, recovery) are applied on top.
//
// Example:
//
//	nats := &config.Nats{
//		Hosts:          []string{"nats://nats1:4222", "nats://nats2:4222"},
//		ConnectTimeout: 5 * time.Second,
//	}
type Nats struct {
	// TLS contains optional TLS configuration for secure connections.
	// If nil, the connection will be made without TLS encryption.
	TLS *TlsClient `yaml:"tls" default:"-"`

	// ConnectionURI is a full NATS connection string (e.g. "nats://user:pass@host:4222").
	// When set, it replaces Hosts and authentication fields.
	// Mutually exclusive with Username, Password, Token, and NkeySeed.
	ConnectionURI Secret `yaml:"connectionUri"`

	// Username is the username for NATS authentication.
	// Used with password-based authentication.
	Username string `yaml:"username"`

	// Token is the authentication token for NATS.
	// Alternative to username/password authentication.
	Token Secret `yaml:"token"`

	// Password is the password for NATS authentication.
	// Used with username-based authentication.
	Password Secret `yaml:"password"`

	// NkeySeed is the NKey seed (private key) for NKey-based authentication.
	// The seed is used to derive the public key and sign server challenges.
	// Mutually exclusive with Token and Username/Password authentication.
	NkeySeed Secret `yaml:"nkeySeed"`

	// ClientName is an optional name for the NATS client connection.
	// Useful for debugging and monitoring purposes.
	ClientName string `yaml:"clientName"`

	// Hosts is the list of NATS server URLs to connect to.
	// Defaults to ["nats://localhost:4222"].
	// The client will attempt to connect to servers in order.
	Hosts []string `yaml:"hosts" default:"nats://localhost:4222"`

	// ConnectTimeout is the maximum time to wait for a connection to be established.
	// Defaults to 5 seconds.
	ConnectTimeout time.Duration `yaml:"connectTimeout" default:"5s"`

	// ReconnectWait is the time to wait between reconnection attempts.
	// Defaults to 10 seconds.
	ReconnectWait time.Duration `yaml:"reconnectWait" default:"10s"`

	// PingInterval is the interval between ping messages to keep the connection alive.
	// Defaults to 10 seconds.
	PingInterval time.Duration `yaml:"pingInterval" default:"10s"`

	// MaxReconnect is the maximum number of reconnection attempts.
	// Set to 0 for unlimited reconnection attempts (default).
	// Set to -1 to disable reconnection.
	MaxReconnect int `yaml:"maxReconnect" default:"0"`

	// MaxPingsOut is the maximum number of ping messages that can be sent
	// without receiving a pong response before the connection is considered dead.
	// Defaults to 3.
	MaxPingsOut int `yaml:"maxPingsOut" default:"3"`

	// Consumers is a collection of consumer configurations keyed by consumer name.
	// Each consumer defines how messages are consumed from NATS JetStream.
	// The key serves as the durable consumer name in NATS.
	Consumers NatsConsumers `yaml:"consumers"`

	// Recovery contains configuration for automatic stream/consumer recovery.
	// If nil, recovery is disabled.
	Recovery *NatsRecovery `yaml:"recovery" default:"-"`
}

// HasConsumers returns true if there are any consumers configured.
func (n *Nats) HasConsumers() bool {
	return len(n.Consumers) > 0
}

// GetConsumerByName returns the consumer configuration for the specified name.
// Returns the consumer configuration and true if found, nil and false otherwise.
func (n *Nats) GetConsumerByName(name string) (*NatsConsumer, bool) {
	if n == nil || n.Consumers == nil {
		return nil, false
	}
	return n.Consumers.GetByName(name)
}

// UseConnectionURI reports whether ConnectionURI is set and should be used
// instead of Hosts and individual authentication fields.
func (n *Nats) UseConnectionURI() bool {
	return !n.ConnectionURI.IsEmpty()
}

// RecoveryEnabled returns true if recovery is configured and enabled.
func (n *Nats) RecoveryEnabled() bool {
	return n.Recovery != nil && n.Recovery.Enabled
}

// DefaultNats returns a Nats configuration with default values.
func DefaultNats() Nats {
	return Nats{
		Hosts:          []string{defaultNatsHost},
		ConnectTimeout: defaultNatsConnectTimeout,
		ReconnectWait:  defaultNatsReconnectWait,
		PingInterval:   defaultNatsPingInterval,
		MaxReconnect:   defaultNatsMaxReconnect,
		MaxPingsOut:    defaultNatsMaxPingsOut,
	}
}

// Validate performs validation on the NATS configuration.
// It validates hosts, TLS configuration, timeout durations, ping settings,
// and all configured consumers to ensure they meet NATS client requirements.
//
// When ConnectionURI is set, Hosts are not required and authentication fields
// (Username, Password, Token, NkeySeed) must not be set (conflict error).
//
// Returns an error if any validation rules fail.
func (n *Nats) Validate() error {
	if err := n.validateConnectionURIConflicts(); err != nil {
		return err
	}

	return ValidateStruct(n,
		validation.Field(&n.Hosts,
			validation.When(!n.UseConnectionURI(),
				validation.Required.Error("minimum one server must be specified"))),
		validation.Field(&n.TLS, validation.When(n.TLS != nil, validation.Required)),
		validation.Field(&n.ConnectTimeout, ozzo_rules.DurationOrZero()),
		validation.Field(&n.ReconnectWait, ozzo_rules.Duration()),
		validation.Field(&n.PingInterval, ozzo_rules.Duration()),
		validation.Field(&n.MaxReconnect, validation.Min(0).Error("must be greater or equal 0")),
		validation.Field(&n.MaxPingsOut, validation.Min(1).Error("must be greater than 1")),
		validation.Field(&n.Consumers, validation.NilOrNotEmpty),
		validation.Field(&n.Recovery, validation.NilOrNotEmpty),
	)
}

// validateConnectionURIConflicts returns an error if ConnectionURI is set
// together with any individual authentication field.
func (n *Nats) validateConnectionURIConflicts() error {
	if !n.UseConnectionURI() {
		return nil
	}

	var errs []error
	if n.Username != "" {
		errs = append(errs, errors.New("username must not be set when connectionUri is used"))
	}
	if !n.Password.IsEmpty() {
		errs = append(errs, errors.New("password must not be set when connectionUri is used"))
	}
	if !n.Token.IsEmpty() {
		errs = append(errs, errors.New("token must not be set when connectionUri is used"))
	}
	if !n.NkeySeed.IsEmpty() {
		errs = append(errs, errors.New("nkeySeed must not be set when connectionUri is used"))
	}

	return errors.Join(errs...)
}
