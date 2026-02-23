// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"cmp"
	"fmt"
	"log/slog"
	"strings"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/nats-io/nkeys"

	"github.com/altessa-s/go-atlas/config"

	corefactory "github.com/altessa-s/go-atlas/core/factory"
	tlsfactory "github.com/altessa-s/go-atlas/security/tlsutils/factory"
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

// Factory creates NATS connections and JetStream consumer configurations
// from [config.Nats] and [config.NatsConsumer]. It delegates TLS setup to
// [tlsfactory.Factory] when configured.
type Factory struct {
	corefactory.Base
	opts       *options
	tlsFactory *tlsfactory.Factory
}

// New creates a new Factory with the given functional options.
// By default the factory uses a discard logger and no TLS factory;
// provide [WithLogger] and [WithTlsFactory] to override.
func New(opts ...Option) *Factory {
	cfg := newOptions(opts...)
	return &Factory{
		Base:       corefactory.NewBase(cfg.logger),
		opts:       cfg,
		tlsFactory: cfg.tlsFactory,
	}
}

// NatsOptionsFromConfig creates [nats.Option] values from configuration.
// It configures timeouts, reconnection behavior (unlimited by default),
// compression, and logging handlers for disconnect/reconnect/error events.
//
// Authentication is selected automatically based on which credential fields
// are set in [config.Nats]: NKey seed, token, or username/password.
// When [config.Nats.ConnectionURI] is set, authentication fields are ignored
// because they are embedded in the URI.
func (f *Factory) NatsOptionsFromConfig(cfg *config.Nats) ([]nats.Option, error) {
	if cfg == nil {
		return nil, fmt.Errorf("configuration is required")
	}
	logger := f.Logger()

	maxReconnect := cmp.Or(cfg.MaxReconnect, UnlimitedReconnects)

	natsOptions := []nats.Option{
		nats.ReconnectBufSize(UnlimitedReconnectBuffer),
		nats.Compression(true),
		nats.RetryOnFailedConnect(true),
		nats.Name(cfg.ClientName),
		nats.Timeout(cfg.ConnectTimeout),
		nats.MaxReconnects(maxReconnect),
		nats.ReconnectWait(cfg.ReconnectWait),
		nats.PingInterval(cfg.PingInterval),
		nats.MaxPingsOutstanding(cfg.MaxPingsOut),
		nats.ReconnectHandler(func(conn *nats.Conn) {
			url := conn.ConnectedUrlRedacted()
			logger.Debug("NATS connection re-established", slog.String("url", url))
		}),
		nats.DisconnectErrHandler(func(_ *nats.Conn, err error) {
			if err != nil {
				logger.Warn("NATS connection lost", slog.Any("error", err))
				return
			}
			logger.Info("NATS connection disconnected")
		}),
		nats.ErrorHandler(func(_ *nats.Conn, subscription *nats.Subscription, err error) {
			if subscription != nil {
				logger.Error("NATS subscription error",
					slog.Any("error", err),
					slog.String("subject", subscription.Subject))
			} else {
				logger.Error("NATS connection error", slog.Any("error", err))
			}
		}),
	}

	if cfg.TLS != nil {
		if f.tlsFactory == nil {
			return nil, f.Errorf("tls config provided but tls factory is not configured")
		}
		tlsConfig, err := f.tlsFactory.CreateClientConfigFromConfig(cfg.TLS)
		if err != nil {
			return nil, f.WrapError(err, "failed to create TLS config")
		}
		if tlsConfig != nil {
			natsOptions = append(natsOptions, nats.Secure(tlsConfig))
		}
	}

	if !cfg.UseConnectionURI() {
		switch {
		case cfg.NkeySeed != "":
			kp, err := nkeys.FromSeed([]byte(cfg.NkeySeed.Expose()))
			if err != nil {
				return nil, f.WrapError(err, "failed to parse nkey seed")
			}
			pub, err := kp.PublicKey()
			if err != nil {
				return nil, f.WrapError(err, "failed to derive nkey public key")
			}
			natsOptions = append(natsOptions, nats.Nkey(pub, kp.Sign))
		case cfg.Token != "":
			natsOptions = append(natsOptions, nats.Token(cfg.Token.Expose()))
		case cfg.Username != "" && cfg.Password != "":
			natsOptions = append(natsOptions, nats.UserInfo(cfg.Username, cfg.Password.Expose()))
		}
	}

	return natsOptions, nil
}

// CreateConnectionFromConfig creates a NATS connection from configuration.
// When [config.Nats.ConnectionURI] is set, it is used directly as the
// connection URL. Otherwise, the URL is built by joining the configured Hosts.
// If a [health.Coordinator] was provided via [WithHealthCoordinator], a health
// checker is registered under the service name "nats".
func (f *Factory) CreateConnectionFromConfig(cfg *config.Nats) (*nats.Conn, error) {
	url := strings.Join(cfg.Hosts, ",")
	if cfg.UseConnectionURI() {
		url = cfg.ConnectionURI.Expose()
	}

	opts, err := f.NatsOptionsFromConfig(cfg)
	if err != nil {
		return nil, err
	}
	conn, err := nats.Connect(url, opts...)
	if err != nil {
		return nil, err
	}

	if f.opts.healthCoordinator != nil {
		f.opts.healthCoordinator.RegisterService("nats", &natsHealthChecker{conn: conn})
	}

	return conn, nil
}

// ConsumerConfigFromConfig converts [config.NatsConsumer] to [jetstream.ConsumerConfig].
// It maps delivery policy, ack policy, replay policy, filter subjects, backoff
// schedules, and performance tuning. When the delivery policy is
// [config.DeliverPolicyByStartSequence] or [config.DeliverPolicyByStartTime],
// the corresponding OptStartSeq or OptStartTime field is set.
func (f *Factory) ConsumerConfigFromConfig(
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
