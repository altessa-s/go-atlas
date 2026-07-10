// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"cmp"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net/url"
	"strings"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/nats-io/nkeys"

	"github.com/altessa-s/go-atlas/config"
	"github.com/altessa-s/go-atlas/observability/health"

	corefactory "github.com/altessa-s/go-atlas/core/factory"
)

const (
	// UnlimitedReconnects indicates that reconnection attempts should continue
	// indefinitely. Passed to [nats.MaxReconnects] when no explicit limit is
	// configured.
	UnlimitedReconnects = -1

	// UnlimitedReconnectBuffer indicates that the reconnect buffer size is
	// unlimited, allowing messages to be buffered during reconnection without
	// dropping. Passed to [nats.ReconnectBufSize].
	UnlimitedReconnectBuffer = -1
)

// Policy conversion maps translate [config] policy constants to their
// [jetstream] equivalents for consumer configuration.
var (
	deliverPolicyMap = map[config.DeliverPolicy]jetstream.DeliverPolicy{
		config.DeliverPolicyAll:             jetstream.DeliverAllPolicy,
		config.DeliverPolicyLast:            jetstream.DeliverLastPolicy,
		config.DeliverPolicyNew:             jetstream.DeliverNewPolicy,
		config.DeliverPolicyByStartSequence: jetstream.DeliverByStartSequencePolicy,
		config.DeliverPolicyByStartTime:     jetstream.DeliverByStartTimePolicy,
	}

	ackPolicyMap = map[config.AckPolicy]jetstream.AckPolicy{
		config.AckPolicyNone:     jetstream.AckNonePolicy,
		config.AckPolicyAll:      jetstream.AckAllPolicy,
		config.AckPolicyExplicit: jetstream.AckExplicitPolicy,
	}

	replayPolicyMap = map[config.ReplayPolicy]jetstream.ReplayPolicy{
		config.ReplayPolicyInstant:  jetstream.ReplayInstantPolicy,
		config.ReplayPolicyOriginal: jetstream.ReplayOriginalPolicy,
	}
)

// natsReconnectHandler logs NATS reconnection. Used by NatsOptions to avoid
// allocating handler logic in hot path; the returned option still captures logger.
func natsReconnectHandler(logger *slog.Logger) func(*nats.Conn) {
	return func(conn *nats.Conn) {
		url := conn.ConnectedUrlRedacted()
		logger.Debug("NATS connection re-established", slog.String("url", url))
	}
}

// natsDisconnectErrHandler logs NATS disconnect events. Used by NatsOptions.
func natsDisconnectErrHandler(logger *slog.Logger) func(*nats.Conn, error) {
	return func(_ *nats.Conn, err error) {
		if err != nil {
			logger.Warn("NATS connection lost", slog.Any("error", err))
			return
		}
		logger.Info("NATS connection disconnected")
	}
}

// natsErrorHandler logs NATS connection and subscription errors. Used by NatsOptions.
func natsErrorHandler(logger *slog.Logger) func(*nats.Conn, *nats.Subscription, error) {
	return func(_ *nats.Conn, subscription *nats.Subscription, err error) {
		if subscription != nil {
			logger.Error("NATS subscription error",
				slog.Any("error", err),
				slog.String("subject", subscription.Subject))
		} else {
			logger.Error("NATS connection error", slog.Any("error", err))
		}
	}
}

// ConnectionBuilder assembles a NATS [nats.Conn] step by step using a fluent API.
// Create instances with [New]. Errors are accumulated and reported at [ConnectionBuilder.Build] time.
// The builder is not safe for concurrent use.
type ConnectionBuilder struct {
	corefactory.Base
	cfg  *config.Nats
	errs []error

	// Dependencies
	tlsConfig         *tls.Config
	healthCoordinator *health.Coordinator
	healthServiceName string
}

// New creates a [ConnectionBuilder] for the given NATS config.
// Config can be nil — the error surfaces at [ConnectionBuilder.Build] time.
func New(cfg *config.Nats) *ConnectionBuilder {
	return &ConnectionBuilder{
		Base: corefactory.NewBase(slog.New(slog.DiscardHandler)),
		cfg:  cfg,
	}
}

// Build creates a NATS connection from configuration.
// When [config.Nats.ConnectionURI] is set, it is used directly as the
// connection URL. Otherwise, the URL is built by joining the configured Hosts.
// If a [health.Coordinator] was provided via [ConnectionBuilder.UseHealthCoordinator],
// a health checker is registered under the service name "nats".
func (b *ConnectionBuilder) Build() (*nats.Conn, error) {
	if err := corefactory.JoinErrors(b.errs); err != nil {
		return nil, err
	}

	if b.cfg == nil {
		return nil, fmt.Errorf("configuration is required")
	}

	url := strings.Join(b.cfg.Hosts, ",")
	if b.cfg.UseConnectionURI() {
		url = b.cfg.ConnectionURI.Expose()
	}

	opts, err := b.NatsOptions()
	if err != nil {
		return nil, err
	}
	conn, err := nats.Connect(url, opts...)
	if err != nil {
		// nats.Connect's error can embed the connection URL verbatim — which
		// may carry inline userinfo (nats://user:pass@host), and it references
		// individual servers, not the comma-joined list. Scrub every raw host
		// occurrence from the error text before returning so credentials cannot
		// leak through whatever log path picks the error up. The wrapped chain is
		// dropped intentionally (callers do not match nats.Connect's internal
		// errors), which is the price of guaranteeing no credential leak.
		return nil, fmt.Errorf("connect to NATS at %s: %s", redactNATSURL(url), scrubURLCredentials(err.Error(), url))
	}

	if b.healthCoordinator != nil {
		b.healthCoordinator.RegisterService(cmp.Or(b.healthServiceName, "nats"), &natsHealthChecker{
			conn:   conn,
			logger: b.Logger(),
		})
	}

	return conn, nil
}

// NatsOptions creates [nats.Option] values from configuration.
// It configures timeouts, reconnection behavior (unlimited by default),
// compression, and logging handlers for disconnect/reconnect/error events.
//
// Authentication is selected automatically based on which credential fields
// are set in [config.Nats]: NKey seed, token, or username/password.
// When [config.Nats.ConnectionURI] is set, authentication fields are ignored
// because they are embedded in the URI.
func (b *ConnectionBuilder) NatsOptions() ([]nats.Option, error) {
	if b.cfg == nil {
		return nil, fmt.Errorf("configuration is required")
	}

	logger := b.Logger()

	maxReconnect := cmp.Or(b.cfg.MaxReconnect, UnlimitedReconnects)

	natsOptions := []nats.Option{
		nats.ReconnectBufSize(UnlimitedReconnectBuffer),
		nats.Compression(true),
		nats.RetryOnFailedConnect(true),
		nats.Name(b.cfg.ClientName),
		nats.Timeout(b.cfg.ConnectTimeout),
		nats.MaxReconnects(maxReconnect),
		nats.ReconnectWait(b.cfg.ReconnectWait),
		nats.PingInterval(b.cfg.PingInterval),
		nats.MaxPingsOutstanding(b.cfg.MaxPingsOut),
		nats.ReconnectHandler(natsReconnectHandler(logger)),
		nats.DisconnectErrHandler(natsDisconnectErrHandler(logger)),
		nats.ErrorHandler(natsErrorHandler(logger)),
	}

	if b.tlsConfig != nil {
		natsOptions = append(natsOptions, nats.Secure(b.tlsConfig))
	}

	if !b.cfg.UseConnectionURI() {
		switch {
		case b.cfg.NkeySeed != "":
			kp, err := nkeys.FromSeed([]byte(b.cfg.NkeySeed.Expose()))
			if err != nil {
				return nil, b.WrapError(err, "failed to parse nkey seed")
			}
			pub, err := kp.PublicKey()
			if err != nil {
				return nil, b.WrapError(err, "failed to derive nkey public key")
			}
			natsOptions = append(natsOptions, nats.Nkey(pub, kp.Sign))
		case b.cfg.Token != "":
			natsOptions = append(natsOptions, nats.Token(b.cfg.Token.Expose()))
		case b.cfg.Username != "" && b.cfg.Password != "":
			natsOptions = append(natsOptions, nats.UserInfo(b.cfg.Username, b.cfg.Password.Expose()))
		}
	}

	return natsOptions, nil
}

// ConsumerConfig converts [config.NatsConsumer] to [jetstream.ConsumerConfig].
// It maps delivery policy, ack policy, replay policy, filter subjects, backoff
// schedules, and performance tuning. When the delivery policy is
// [config.DeliverPolicyByStartSequence] or [config.DeliverPolicyByStartTime],
// the corresponding OptStartSeq or OptStartTime field is set.
func ConsumerConfig(
	cfg *config.NatsConsumer,
) (*jetstream.ConsumerConfig, error) {
	if cfg == nil {
		return nil, fmt.Errorf("configuration is required")
	}
	consumerCfg := &jetstream.ConsumerConfig{
		Description:       cfg.Description,
		FilterSubjects:    cfg.FilterSubjects,
		MaxAckPending:     cfg.MaxAckPending,
		MaxWaiting:        cfg.MaxWaiting,
		InactiveThreshold: cfg.InactiveThreshold,
		AckWait:           cfg.AckWait,
		MaxDeliver:        cfg.MaxDeliver,
		BackOff:           cfg.BackOff,
		DeliverPolicy:     deliverPolicyMap[cfg.DeliverPolicy],
		AckPolicy:         ackPolicyMap[cfg.AckPolicy],
		ReplayPolicy:      replayPolicyMap[cfg.ReplayPolicy],
		Durable:           cfg.DurableName,
		Name:              cfg.DurableName,
	}

	if cfg.DeliverPolicy == config.DeliverPolicyByStartSequence {
		consumerCfg.OptStartSeq = cfg.OptStartSeq
	}

	if cfg.DeliverPolicy == config.DeliverPolicyByStartTime {
		consumerCfg.OptStartTime = cfg.OptStartTime
	}

	return consumerCfg, nil
}

// redactNATSURL strips userinfo (user[:password]@) from a NATS
// connection string before it is logged or returned in an error.
// Multi-URL comma-separated lists are handled — each component is
// redacted independently. Inputs that don't parse as a URL are
// returned unchanged (the redaction only applies when there's userinfo
// to strip).
func redactNATSURL(raw string) string {
	parts := strings.Split(raw, ",")
	for i, p := range parts {
		u, err := url.Parse(strings.TrimSpace(p))
		if err != nil || u.User == nil {
			continue
		}
		u.User = nil
		parts[i] = u.String()
	}
	return strings.Join(parts, ",")
}

// scrubURLCredentials removes inline userinfo from every occurrence of a raw host
// (taken from the comma-separated url) in msg, replacing it with the redacted
// form. It is used to sanitize error strings that may echo a connection URL
// carrying credentials.
func scrubURLCredentials(msg, url string) string {
	for raw := range strings.SplitSeq(url, ",") {
		if raw = strings.TrimSpace(raw); raw != "" {
			msg = strings.ReplaceAll(msg, raw, redactNATSURL(raw))
		}
	}
	return msg
}
