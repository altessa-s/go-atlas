// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config

import (
	"fmt"
	"time"

	ozzo_rules "github.com/altessa-s/ozzo-rules"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// NatsConsumers represents a collection of NATS JetStream consumer configurations.
// It is a map where keys are consumer names (durable names) and values are consumer configurations.
type NatsConsumers map[string]*NatsConsumer

// GetByName returns the consumer configuration for the specified name.
// Returns the consumer configuration and true if found, nil and false otherwise.
func (nc NatsConsumers) GetByName(name string) (*NatsConsumer, bool) {
	consumer, found := nc[name]
	return consumer, found
}

// DeliverPolicy defines the point in the stream to start delivering messages.
type DeliverPolicy string

const (
	// DeliverPolicyAll delivers all available messages from the stream.
	DeliverPolicyAll DeliverPolicy = "all"

	// DeliverPolicyLast delivers starting with the last message in the stream.
	DeliverPolicyLast DeliverPolicy = "last"

	// DeliverPolicyNew delivers only new messages that arrive after consumer creation.
	DeliverPolicyNew DeliverPolicy = "new"

	// DeliverPolicyByStartSequence delivers starting from a specific sequence number.
	DeliverPolicyByStartSequence DeliverPolicy = "by_start_sequence"

	// DeliverPolicyByStartTime delivers starting from a specific time.
	DeliverPolicyByStartTime DeliverPolicy = "by_start_time"
)

// AckPolicy defines how messages should be acknowledged.
type AckPolicy string

const (
	// AckPolicyNone requires no acknowledgement.
	// Messages are considered delivered immediately.
	AckPolicyNone AckPolicy = "none"

	// AckPolicyAll acknowledges all messages up to and including the current one.
	AckPolicyAll AckPolicy = "all"

	// AckPolicyExplicit requires explicit acknowledgement for each message.
	AckPolicyExplicit AckPolicy = "explicit"
)

// ReplayPolicy defines how messages are replayed from the stream.
type ReplayPolicy string

const (
	// ReplayPolicyInstant replays messages as fast as possible.
	ReplayPolicyInstant ReplayPolicy = "instant"

	// ReplayPolicyOriginal replays messages at the original rate they were received.
	ReplayPolicyOriginal ReplayPolicy = "original"
)

// NatsConsumer represents the configuration for a NATS JetStream consumer.
// Consumers are clients that consume messages from a stream with specific delivery
// and acknowledgement policies.
type NatsConsumer struct {
	// Description is an optional description of the consumer.
	Description string `yaml:"description"`

	// DurableName is the durable name for the consumer.
	// If not specified, the consumer name from the configuration map key is used.
	// Durable consumers maintain their state across restarts and reconnections.
	DurableName string `yaml:"durableName"`

	// DeliverPolicy defines the point in the stream to start delivering messages.
	// Defaults to "all" which delivers all available messages.
	DeliverPolicy DeliverPolicy `yaml:"deliverPolicy" default:"all"`

	// OptStartSeq is the sequence number to start from when DeliverPolicy is "by_start_sequence".
	// Must be greater than 0 when used.
	OptStartSeq uint64 `yaml:"optStartSeq"`

	// OptStartTime is the time to start from when DeliverPolicy is "by_start_time".
	// Format: RFC3339 timestamp.
	OptStartTime *time.Time `yaml:"optStartTime"`

	// AckPolicy defines how messages should be acknowledged.
	// Defaults to "explicit" which requires explicit ack for each message.
	AckPolicy AckPolicy `yaml:"ackPolicy" default:"explicit"`

	// AckWait is the duration to wait for acknowledgement before redelivery.
	// Defaults to 30 seconds.
	AckWait time.Duration `yaml:"ackWait" default:"30s"`

	// MaxDeliver is the maximum number of delivery attempts for a message.
	// After this, the message is considered permanently failed.
	// Set to -1 for unlimited attempts. Defaults to -1.
	MaxDeliver int `yaml:"maxDeliver" default:"-1"`

	// BackOff is a custom backoff schedule for message redelivery.
	// If not specified, uses exponential backoff.
	// Each duration in the slice represents the delay before the next redelivery attempt.
	// Example: [1s, 5s, 30s, 2m, 5m] means:
	//   - 1st retry after 1 second
	//   - 2nd retry after 5 seconds
	//   - 3rd retry after 30 seconds
	//   - 4th retry after 2 minutes
	//   - 5th retry after 5 minutes
	// After exhausting the backoff schedule, the last duration is used for subsequent retries.
	BackOff []time.Duration `yaml:"backOff"`

	// FilterSubject is an optional subject filter for the consumer.
	// Only messages matching this subject will be delivered.
	FilterSubjects []string `yaml:"filterSubject"`

	// ReplayPolicy defines how messages are replayed from the stream.
	// Defaults to "instant" which replays as fast as possible.
	ReplayPolicy ReplayPolicy `yaml:"replayPolicy" default:"instant"`

	// MaxAckPending is the maximum number of messages that can be outstanding
	// (delivered but not yet acknowledged) at any time.
	// Defaults to 1000.
	MaxAckPending int `yaml:"maxAckPending" default:"1000"`

	// MaxWaiting is the maximum number of waiting pull requests.
	// Only applicable for pull-based consumers.
	// Defaults to 512.
	MaxWaiting int `yaml:"maxWaiting" default:"512"`

	// InactiveThreshold is the duration of inactivity before the consumer is removed.
	// Set to 0 to disable automatic removal.
	// Defaults to 0 (disabled).
	InactiveThreshold time.Duration `yaml:"inactiveThreshold" default:"0s"`
}

// Validate performs validation on the NatsConsumer configuration.
// It validates delivery policies, acknowledgement settings, timeout durations,
// and backoff schedules to ensure they meet NATS JetStream consumer requirements.
//
// Returns an error if any validation rules fail.
func (nc *NatsConsumer) Validate() error {
	return ValidateStruct(nc,
		validation.Field(&nc.DeliverPolicy,
			ozzo_rules.OneOf(DeliverPolicyAll, DeliverPolicyLast, DeliverPolicyNew, DeliverPolicyByStartSequence, DeliverPolicyByStartTime)),
		validation.Field(&nc.OptStartSeq,
			validation.When(nc.DeliverPolicy == DeliverPolicyByStartSequence,
				validation.Required.Error("optStartSeq is required when deliverPolicy is by_start_sequence"),
				validation.Min(uint64(1)).Error("must be greater than 0"))),
		validation.Field(&nc.OptStartTime,
			validation.When(nc.DeliverPolicy == DeliverPolicyByStartTime,
				validation.Required.Error("optStartTime is required when deliverPolicy is by_start_time"))),
		validation.Field(&nc.AckPolicy, ozzo_rules.OneOf(AckPolicyNone, AckPolicyAll, AckPolicyExplicit)),
		validation.Field(&nc.AckWait, validation.When(nc.AckPolicy != AckPolicyNone, ozzo_rules.Duration())),
		validation.Field(&nc.MaxDeliver, validation.Min(-1).Error("must be -1 (unlimited) or greater")),
		validation.Field(&nc.BackOff, validation.By(validateBackOff)),
		validation.Field(&nc.ReplayPolicy, ozzo_rules.OneOf(ReplayPolicyInstant, ReplayPolicyOriginal)),
		validation.Field(&nc.MaxAckPending, validation.Min(1).Error("must be greater than 0")),
		validation.Field(&nc.MaxWaiting, validation.Min(1).Error("must be greater than 0")),
		validation.Field(&nc.InactiveThreshold, ozzo_rules.DurationOrZero()),
	)
}

// validateBackOff validates the BackOff field.
// It ensures all durations in the backoff schedule are positive.
func validateBackOff(value any) error {
	backOff, ok := value.([]time.Duration)
	if !ok {
		return nil // Skip validation if not the expected type
	}

	for i, duration := range backOff {
		if duration <= 0 {
			return validation.NewError("validation_backoff_duration",
				fmt.Sprintf("backOff[%d] must be greater than 0", i))
		}
	}

	return nil
}
