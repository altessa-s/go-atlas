// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package natsprovider

import (
	"context"
	"fmt"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/altessa-s/go-atlas-core/runtime/appinfo"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
)

// consumerFilters returns the subject filters of cfg, or nil when the
// consumer has none and therefore reads the whole stream.
func consumerFilters(cfg *jetstream.ConsumerConfig) []string {
	if len(cfg.FilterSubjects) > 0 {
		return cfg.FilterSubjects
	}
	if cfg.FilterSubject != "" {
		return []string{cfg.FilterSubject}
	}
	return nil
}

// checkConsumerScope verifies that a consumer with the given filters delivers
// only subjects inside both the subscription topic and the allowlist. A
// consumer without filters reads the whole stream, so every subject the
// stream captures must be covered by the allowlist. It is a no-op without an
// allowlist.
func (n *NATS) checkConsumerScope(topic string, filters, streamSubjects []string) error {
	if len(n.allowedSubjects) == 0 {
		return nil
	}
	if len(filters) == 0 {
		for _, subject := range streamSubjects {
			if err := n.checkSubscriptionAllowed(subject); err != nil {
				return coreerrs.Wrap(err, "consumer without a subject filter reads the whole stream")
			}
		}
		return nil
	}
	for _, filter := range filters {
		if !subscriptionCovered(topic, filter) {
			return coreerrs.Wrapf(ErrSubjectNotAllowed, "consumer filter %q is wider than subscription %q", filter, topic)
		}
		if err := n.checkSubscriptionAllowed(filter); err != nil {
			return err
		}
	}
	return nil
}

// resolveConsumer creates or looks up the consumer for a subscription to
// topic and refuses one whose effective subject scope escapes the allowlist.
// The proposed scope is validated before a consumer is created or updated, a
// named consumer is inspected before use, and the server-side filters of the
// resulting consumer are validated again before consuming.
func (ss *streamSubscriber) resolveConsumer(ctx context.Context, stream jetstream.Stream, streamName, topic string) (jetstream.Consumer, error) {
	var (
		consumer jetstream.Consumer
		err      error
	)
	streamSubjects := stream.CachedInfo().Config.Subjects

	switch {
	case ss.consumerConfig != nil:
		cfg := *ss.consumerConfig
		if len(ss.provider.allowedSubjects) > 0 && len(consumerFilters(&cfg)) == 0 {
			cfg.FilterSubject = topic
		}
		if err = ss.provider.checkConsumerScope(topic, consumerFilters(&cfg), streamSubjects); err != nil {
			return nil, err
		}
		consumer, err = stream.CreateOrUpdateConsumer(ctx, cfg)
		if err != nil {
			return nil, coreerrs.Wrapf(err, "failed to create/update consumer for stream '%s'", streamName)
		}
	case ss.consumerName != "":
		consumer, err = stream.Consumer(ctx, ss.consumerName)
		if err != nil {
			return nil, coreerrs.Wrapf(err, "failed to get consumer '%s' on stream '%s'", ss.consumerName, streamName)
		}
	default:
		// FilterSubject scopes the consumer to the handler's subject: without
		// it the consumer reads the whole stream, so a subscription to
		// orders.*.created would also receive every other orders.> message.
		consumer, err = stream.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{
			FilterSubject:     topic,
			DeliverPolicy:     jetstream.DeliverNewPolicy, // only messages published after Subscribe
			InactiveThreshold: EphemeralInactiveThreshold,
			Description:       fmt.Sprintf("Ephemeral consumer for service '%s'", appinfo.Name),
		})
		if err != nil {
			return nil, coreerrs.Wrapf(err, "failed to create/update ephemeral consumer for stream '%s'", streamName)
		}
	}

	info := consumer.CachedInfo()
	if err = ss.provider.checkConsumerScope(topic, consumerFilters(&info.Config), streamSubjects); err != nil {
		return nil, err
	}
	return consumer, nil
}
