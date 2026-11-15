// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/factory/messaging/broker"
	"github.com/altessa-s/go-atlas/messaging/broker"
	"github.com/altessa-s/go-atlas/messaging/broker/msg"
	"github.com/altessa-s/go-atlas/reliability/outbox"

	brokerconfig "github.com/altessa-s/go-atlas/config/broker"
	corefactory "github.com/altessa-s/go-atlas/factory/core"
)

var errBuild = errors.New("build failed")

// TestCreateInProgressManager covers the config-gating and scheduler-wiring
// branches of BrokerBuilder.CreateInProgressManager.
func TestCreateInProgressManager(t *testing.T) {
	t.Parallel()

	t.Run("nil_config_is_an_error", func(t *testing.T) {
		t.Parallel()
		_, err := factory.New(nil).CreateInProgressManager()
		require.Error(t, err)
	})

	t.Run("disabled_returns_nothing", func(t *testing.T) {
		t.Parallel()
		cfg := &brokerconfig.Config{InProgress: brokerconfig.InProgress{Enabled: false}}
		mgr, err := factory.New(cfg).CreateInProgressManager()
		require.NoError(t, err)
		require.Nil(t, mgr)
	})

	t.Run("enabled_without_scheduler_builds_a_manager", func(t *testing.T) {
		t.Parallel()
		cfg := &brokerconfig.Config{InProgress: brokerconfig.InProgress{Enabled: true, TickSchedule: "@every 1s"}}
		mgr, err := factory.New(cfg).CreateInProgressManager()
		require.NoError(t, err)
		require.NotNil(t, mgr)
	})

	t.Run("enabled_with_scheduler_registers_the_tick_task", func(t *testing.T) {
		t.Parallel()
		reg := &capturingRegistrar{}
		cfg := &brokerconfig.Config{InProgress: brokerconfig.InProgress{Enabled: true, TickSchedule: "@every 1s"}}
		mgr, err := factory.New(cfg).UseScheduler(reg).CreateInProgressManager()
		require.NoError(t, err)
		require.NotNil(t, mgr)
		require.Equal(t, []string{"broker-inprogress-heartbeat"}, reg.ids())
	})
}

// stubProvider is a minimal broker.Provider used to exercise
// BrokerBuilder.Build without a real transport.
type stubProvider struct{}

func (stubProvider) Publish(context.Context, msg.Message) error         { return nil }
func (stubProvider) PublishBatch(context.Context, ...msg.Message) error { return nil }
func (stubProvider) Subscriber(broker.SubscriberFactory) broker.Subscriber {
	return nil
}

// TestBuild covers BrokerBuilder.Build and its applyBrokerDefaults helper,
// including how UsePublishConverter reaches the assembled Broker.
func TestBuild(t *testing.T) {
	t.Parallel()

	t.Run("nil_config_is_an_error", func(t *testing.T) {
		t.Parallel()
		_, err := factory.New(nil).Build(stubProvider{})
		require.ErrorIs(t, err, corefactory.ErrConfigRequired)
	})

	t.Run("missing_provider_is_a_dependency_error", func(t *testing.T) {
		t.Parallel()
		_, err := factory.New(&brokerconfig.Config{}).Build(nil)
		require.Error(t, err)
	})

	t.Run("without_a_converter_PublishAny_errors", func(t *testing.T) {
		t.Parallel()
		b, err := factory.New(&brokerconfig.Config{}).Build(stubProvider{})
		require.NoError(t, err)
		require.ErrorIs(t, b.PublishAny(t.Context(), "anything"), broker.ErrPublishConverterNotSet)
	})

	t.Run("applies_the_configured_publish_converter", func(t *testing.T) {
		t.Parallel()
		converter := func(any) (msg.Message, error) { return *msg.NewMessage("x", []byte("y")), nil }
		b, err := factory.New(&brokerconfig.Config{}).UsePublishConverter(converter).Build(stubProvider{})
		require.NoError(t, err)
		require.NoError(t, b.PublishAny(t.Context(), "anything"))
	})
}

func TestCreateProvider(t *testing.T) {
	t.Parallel()
	cfg := &brokerconfig.Config{Provider: brokerconfig.ProviderNATS}

	t.Run("nil_config_is_an_error", func(t *testing.T) {
		t.Parallel()
		_, err := factory.New(nil).CreateProvider()
		require.Error(t, err)
	})

	t.Run("unregistered_nats_fails", func(t *testing.T) {
		t.Parallel()
		_, err := factory.New(cfg).CreateProvider()
		require.Error(t, err)
	})

	t.Run("unsupported_type_fails", func(t *testing.T) {
		t.Parallel()
		_, err := factory.New(&brokerconfig.Config{Provider: "kafka"}).CreateProvider()
		require.Error(t, err)
	})

	t.Run("nil_function_fails", func(t *testing.T) {
		t.Parallel()
		_, err := factory.New(cfg).UseProvider(brokerconfig.ProviderNATS, nil).CreateProvider()
		require.Error(t, err)
	})

	t.Run("function_error_propagates", func(t *testing.T) {
		t.Parallel()
		failing := func(*brokerconfig.Config, factory.ProviderEnv) (broker.Provider, error) { return nil, errBuild }
		_, err := factory.New(cfg).UseProvider(brokerconfig.ProviderNATS, failing).CreateProvider()
		require.ErrorIs(t, err, errBuild)
	})

	t.Run("registered_function_is_used", func(t *testing.T) {
		t.Parallel()
		logger := slog.New(slog.DiscardHandler)
		reg := &capturingRegistrar{}
		want := &stubProvider{}
		registered := func(got *brokerconfig.Config, env factory.ProviderEnv) (broker.Provider, error) {
			require.Same(t, cfg, got)
			require.Same(t, logger, env.Logger)
			require.Same(t, reg, env.Scheduler)
			return want, nil
		}
		failing := func(*brokerconfig.Config, factory.ProviderEnv) (broker.Provider, error) { return nil, errBuild }
		provider, err := factory.New(cfg).
			UseLogger(logger).
			UseScheduler(reg).
			UseProvider(brokerconfig.ProviderNATS, failing).
			UseProvider(brokerconfig.ProviderNATS, registered).
			CreateProvider()
		require.NoError(t, err)
		require.Same(t, want, provider, "a later registration replaces the earlier one")
	})
}

func TestCreateOutboxWithStorage(t *testing.T) {
	t.Parallel()

	_, err := factory.New(outboxConfig()).CreateOutboxWithStorage(t.Context(), nil, stubPublisher{})
	require.Error(t, err)

	failing := func(context.Context, *brokerconfig.Outbox, *slog.Logger) (outbox.Storage, error) { return nil, errBuild }
	_, err = factory.New(outboxConfig()).CreateOutboxWithStorage(t.Context(), failing, stubPublisher{})
	require.ErrorIs(t, err, errBuild)
}
