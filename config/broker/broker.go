// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package brokerconfig

import (
	natsconfig "github.com/altessa-s/go-atlas/config/nats"
	validationconfig "github.com/altessa-s/go-atlas/config/validation"
	ozzo_rules "github.com/altessa-s/ozzo-rules"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// Default values for Broker configuration.
const (
	defaultBrokerProvider = ProviderNATS
)

// Provider defines the type of message broker to use.
// Currently, only NATS is supported as a broker provider.
type Provider string

const (
	// ProviderNATS represents the NATS broker provider.
	ProviderNATS Provider = "nats"
)

// Config defines the configuration for message broker operations.
// Supports multiple broker providers with provider-specific settings.
//
// Example:
//
//	broker := &brokerconfig.Config{Provider: brokerconfig.ProviderNATS}
type Config struct {
	// Provider is the type of broker to use.
	Provider Provider `yaml:"provider" default:"nats"`

	// Nats contains NATS-specific configuration.
	// Required when Provider is "nats".
	NATS *natsconfig.Config `yaml:"nats"`

	// InProgress contains configuration for InProgress heartbeat manager.
	InProgress InProgress `yaml:"inProgress"`

	// Outbox contains configuration for reliable message delivery.
	Outbox Outbox `yaml:"outbox"`
}

// Default returns a Broker configuration with default values.
func Default() Config {
	return Config{
		Provider: defaultBrokerProvider,
		InProgress: InProgress{
			Enabled:                  defaultInProgressEnabled,
			TickSchedule:             defaultInProgressTickSchedule,
			DefaultHeartbeatInterval: defaultInProgressHeartbeatInterval,
			MaxEntries:               defaultInProgressMaxEntries,
			Metrics: InProgressMetrics{
				Enabled: defaultInProgressMetricsEnabled,
				Prefix:  defaultInProgressMetricsPrefix,
			},
		},
		Outbox: Outbox{
			Enabled:                 defaultOutboxEnabled,
			FetchTimeout:            defaultOutboxFetchTimeout,
			HandleTimeout:           defaultOutboxHandleTimeout,
			UpdateTimeout:           defaultOutboxUpdateTimeout,
			MessagesBatchSize:       defaultOutboxMessagesBatchSize,
			RetryMaxAttempts:        defaultOutboxRetryMaxAttempts,
			PublishedEventsLifetime: defaultOutboxPublishedEventsLifetime,
			TopicCompaction:         defaultOutboxTopicCompaction,
		},
	}
}

// Validate performs validation of the Broker configuration.
// Returns an error if validation fails, nil otherwise.
func (b *Config) Validate() error {
	return validationconfig.ValidateStruct(b,
		validation.Field(&b.Provider, validation.Required, ozzo_rules.OneOf(ProviderNATS)),
		validation.Field(&b.NATS, validation.When(b.Provider == ProviderNATS, validation.Required)),
		validation.Field(&b.InProgress),
		validation.Field(&b.Outbox),
	)
}
