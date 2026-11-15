// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"errors"

	"github.com/nats-io/nats.go"

	"github.com/altessa-s/go-atlas-core/collections/slices"
	"github.com/altessa-s/go-atlas/messaging/broker"
	"github.com/altessa-s/go-atlas/messaging/broker/providers/nats/recovery"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
	brokerconfig "github.com/altessa-s/go-atlas/config/broker"
	corefactory "github.com/altessa-s/go-atlas/factory/core"
	brokerfactory "github.com/altessa-s/go-atlas/factory/messaging/broker"
	natsprovider "github.com/altessa-s/go-atlas/messaging/broker/providers/nats"
)

var (
	// ErrConnRequired is returned when the NATS provider is built without a
	// connection.
	ErrConnRequired = errors.New("broker: nats connection is required for provider nats")
	// ErrProviderRequired is returned when the recovery manager is built
	// without a NATS provider.
	ErrProviderRequired = errors.New("broker: nats provider is required for stream recovery")
)

// ProviderWithRecovery bundles a [natsprovider.NATS] provider with its
// optional [recovery.Manager]. Call [ProviderWithRecovery.Close] when the
// result is no longer needed to stop the recovery manager.
type ProviderWithRecovery struct {
	// Provider is the NATS broker provider.
	Provider *natsprovider.NATS
	// Recovery is the optional recovery manager (nil if recovery is disabled).
	Recovery *recovery.Manager
}

// Close stops the recovery manager if it was started.
// The NATS provider should be closed separately via its connection.
func (n *ProviderWithRecovery) Close() {
	if n.Recovery != nil {
		n.Recovery.Close()
	}
}

// Builder returns the provider function of the nats type, ready for
// brokerfactory.BrokerBuilder.UseProvider. The provider publishes and
// subscribes through conn; the caller owns it. Use [NewWithRecovery] when the
// configuration enables stream recovery.
func Builder(conn *nats.Conn) brokerfactory.ProviderFunc {
	return func(cfg *brokerconfig.Config, env brokerfactory.ProviderEnv) (broker.Provider, error) {
		provider, err := newProvider(conn, cfg, env)
		if err != nil {
			return nil, err
		}
		return provider, nil
	}
}

// NewWithRecovery creates a NATS provider with optional recovery manager
// from the broker configuration. If recovery is enabled, the recovery manager
// is created and will register background tasks with env.Scheduler if one is
// provided.
//
// The recovery manager is NOT started automatically. Call Recovery.Start() after
// registering streams to begin advisory event listening.
//
// Example:
//
//	result, err := natsfactory.NewWithRecovery(natsConn, &cfg.Broker,
//	    brokerfactory.ProviderEnv{Logger: logger, Scheduler: scheduler})
//	if err != nil {
//	    return err
//	}
//	defer result.Close()
//
//	// Register streams for recovery before starting
//	if result.Recovery != nil {
//	    result.Recovery.RegisterStream(streamCfg)
//	    result.Recovery.Start()
//	}
func NewWithRecovery(conn *nats.Conn, cfg *brokerconfig.Config, env brokerfactory.ProviderEnv) (*ProviderWithRecovery, error) {
	provider, err := newProvider(conn, cfg, env)
	if err != nil {
		return nil, err
	}

	recoveryMgr, err := NewRecoveryManager(provider, cfg, env)
	if err != nil {
		return nil, err
	}

	return &ProviderWithRecovery{
		Provider: provider,
		Recovery: recoveryMgr,
	}, nil
}

// NewRecoveryManager creates a NATS JetStream recovery manager from the
// broker configuration. The recovery manager automatically recovers deleted
// streams and consumers.
//
// A nil configuration is an error. If [brokerconfig.Config.NATS] is nil, or
// recovery is not enabled, returns nil, nil.
//
// The manager will register recovery tasks (health check, stale cleanup) if
// env.Scheduler is set.
func NewRecoveryManager(provider *natsprovider.NATS, cfg *brokerconfig.Config, env brokerfactory.ProviderEnv) (*recovery.Manager, error) {
	if cfg == nil {
		return nil, corefactory.ErrConfigRequired
	}

	natsCfg := cfg.NATS
	if natsCfg == nil || natsCfg.Recovery == nil || !natsCfg.Recovery.Enabled {
		return nil, nil //nolint:nilnil
	}

	if provider == nil {
		return nil, ErrProviderRequired
	}

	recoveryCfg := natsCfg.Recovery

	// Build options from configuration.
	opts := []recovery.Option{
		recovery.WithMaxRecoveryAttempts(recoveryCfg.MaxRecoveryAttempts),
		recovery.WithRecoveryBackoff(recoveryCfg.RecoveryBackoff),
		recovery.WithStaleRecoveryTimeout(recoveryCfg.StaleRecoveryTimeout),
	}
	opts = slices.AppendIf(opts, env.Logger != nil, recovery.WithLogger(env.Logger))

	// Add scheduler and schedule options if scheduler is available
	opts = slices.AppendIf(opts, env.Scheduler != nil,
		recovery.WithScheduler(env.Scheduler),
		recovery.WithHealthCheckSchedule(recoveryCfg.HealthCheckSchedule),
		recovery.WithStaleRecoveryCleanupSchedule(recoveryCfg.StaleRecoveryCleanupSchedule),
	)

	// Create recovery manager.
	manager, err := recovery.New(provider, opts...)
	if err != nil {
		return nil, coreerrs.Wrap(err, "failed to create recovery manager")
	}

	return manager, nil
}

// newProvider creates a NATS broker provider from an existing connection,
// with the subscriber drain timeout of the broker's NATS configuration and
// the subscriber metrics on env.Meter when one is set.
func newProvider(conn *nats.Conn, cfg *brokerconfig.Config, env brokerfactory.ProviderEnv) (*natsprovider.NATS, error) {
	if conn == nil {
		return nil, ErrConnRequired
	}

	var opts []natsprovider.Option
	opts = slices.AppendIf(opts, env.Meter != nil, natsprovider.WithMeter(env.Meter))
	opts = slices.AppendIfFunc(opts, cfg != nil && cfg.NATS != nil, func() []natsprovider.Option {
		return []natsprovider.Option{natsprovider.WithDrainTimeout(cfg.NATS.SubscriberDrainTimeout)}
	})

	provider, err := natsprovider.New(conn, opts...)
	if err != nil {
		return nil, coreerrs.Wrap(err, "failed to create NATS provider")
	}

	return provider, nil
}
