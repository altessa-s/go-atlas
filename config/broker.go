// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config

import (
	ozzo_rules "github.com/altessa-s/ozzo-rules"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// Default values for Broker configuration.
const (
	defaultBrokerProvider = BrokerProviderNats
)

// BrokerProvider defines the type of message broker to use.
// Currently, only NATS is supported as a broker provider.
type BrokerProvider string

const (
	// BrokerProviderNats represents the NATS broker provider.
	BrokerProviderNats BrokerProvider = "nats"
)

// Broker defines the configuration for message broker operations.
// Supports multiple broker providers with provider-specific settings.
//
// Example:
//
//	broker := &config.Broker{Provider: config.BrokerProviderNats}
type Broker struct {
	// Provider is the type of broker to use.
	Provider BrokerProvider `yaml:"provider" default:"nats"`

	// Nats contains NATS-specific configuration.
	// Required when Provider is "nats".
	Nats *Nats `yaml:"nats"`

	// InProgress contains configuration for InProgress heartbeat manager.
	InProgress InProgress `yaml:"inProgress"`

	// Outbox contains configuration for reliable message delivery.
	Outbox Outbox `yaml:"outbox"`
}

// DefaultBroker returns a Broker configuration with default values.
func DefaultBroker() Broker {
	return Broker{
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
func (b *Broker) Validate() error {
	return ValidateStruct(b,
		validation.Field(&b.Provider, validation.Required, ozzo_rules.OneOf(BrokerProviderNats)),
		validation.Field(&b.Nats, validation.When(b.Provider == BrokerProviderNats, validation.Required)),
		validation.Field(&b.InProgress),
		validation.Field(&b.Outbox),
	)
}
