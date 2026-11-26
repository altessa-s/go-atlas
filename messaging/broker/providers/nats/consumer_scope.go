// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package natsprovider

import (
	"cmp"
	"context"
	"errors"
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
// only subjects inside both the subscription topic and the allowlist. It is a
// no-op without an allowlist.
//
// A consumer without filters reads the whole stream. That is allowed only
// when the delivery scope is provable: the stream is a plain one (no sources,
// mirror or subject transform, which bring in subjects its Subjects list does
// not show) and every subject it captures lies inside the topic and the
// allowlist. Anything else is refused.
func (n *NATS) checkConsumerScope(topic string, filters []string, stream *jetstream.StreamConfig) error {
	if len(n.allowedSubjects) == 0 {
		return nil
	}
	if len(filters) == 0 {
		return n.checkWholeStream(topic, stream)
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

// checkWholeStream refuses unfiltered access to a stream unless its scope is
// provably inside the topic and the allowlist.
func (n *NATS) checkWholeStream(topic string, stream *jetstream.StreamConfig) error {
	if len(stream.Subjects) == 0 || stream.Mirror != nil || len(stream.Sources) > 0 || stream.SubjectTransform != nil {
		return coreerrs.Wrapf(ErrSubjectNotAllowed,
			"consumer without a subject filter reads stream %q whose subjects cannot be established", stream.Name)
	}
	for _, subject := range stream.Subjects {
		if !subscriptionCovered(topic, subject) {
			return coreerrs.Wrapf(ErrSubjectNotAllowed,
				"consumer without a subject filter reads stream subject %q outside subscription %q", subject, topic)
		}
		if err := n.checkSubscriptionAllowed(subject); err != nil {
			return err
		}
	}
	return nil
}

// resolveConsumer creates or looks up the consumer for a subscription to
// topic and refuses one whose effective subject scope escapes the allowlist.
// With an allowlist, an existing named or durable consumer is looked up and
// used as it is, never updated; a custom config needs a subject filter; and
// the server-side filters of the resulting consumer are validated before
// consuming.
func (ss *streamSubscriber) resolveConsumer(ctx context.Context, stream jetstream.Stream, streamName, topic string) (jetstream.Consumer, error) {
	var (
		consumer jetstream.Consumer
		err      error
	)
	streamCfg := &stream.CachedInfo().Config
	restricted := len(ss.provider.allowedSubjects) > 0

	switch {
	case ss.consumerConfig != nil:
		cfg := *ss.consumerConfig
		if restricted {
			filters := consumerFilters(&cfg)
			if len(filters) == 0 {
				return nil, coreerrs.Wrapf(ErrSubjectNotAllowed, "consumer config for subscription %q needs a subject filter when an allowlist is set", topic)
			}
			if err = ss.provider.checkConsumerScope(topic, filters, streamCfg); err != nil {
				return nil, err
			}
			// A durable is shared by every client using it: look it up first
			// and never rewrite it.
			if name := cmp.Or(cfg.Durable, cfg.Name); name != "" {
				consumer, err = stream.Consumer(ctx, name)
				if err != nil && !errors.Is(err, jetstream.ErrConsumerNotFound) {
					return nil, coreerrs.Wrapf(err, "failed to get consumer '%s' on stream '%s'", name, streamName)
				}
			}
		}
		if consumer == nil {
			consumer, err = stream.CreateOrUpdateConsumer(ctx, cfg)
			if err != nil {
				return nil, coreerrs.Wrapf(err, "failed to create/update consumer for stream '%s'", streamName)
			}
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
	if err = ss.provider.checkConsumerScope(topic, consumerFilters(&info.Config), streamCfg); err != nil {
		return nil, err
	}
	return consumer, nil
}
