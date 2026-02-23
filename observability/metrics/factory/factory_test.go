// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/config"
	"github.com/altessa-s/go-atlas/observability/metrics"
	"github.com/altessa-s/go-atlas/observability/metrics/factory"
)

func TestFactory_CreateFromConfig_Disabled(t *testing.T) {
	f := factory.New()

	// Nil config
	collector, err := f.CreateFromConfig(nil)
	if err != nil {
		t.Fatalf("CreateFromConfig(nil) error = %v", err)
	}
	if !metrics.IsNoop(collector) {
		t.Error("expected Noop collector for nil config")
	}

	// Disabled config
	cfg := &config.Metrics{Enable: false}
	collector, err = f.CreateFromConfig(cfg)
	if err != nil {
		t.Fatalf("CreateFromConfig(disabled) error = %v", err)
	}
	if !metrics.IsNoop(collector) {
		t.Error("expected Noop collector for disabled config")
	}
}

func TestFactory_CreateFromConfig_Prometheus(t *testing.T) {
	f := factory.New()

	cfg := &config.Metrics{
		Enable:      true,
		Type:        config.MetricsTypePrometheus,
		ServiceName: "test",
	}

	collector, err := f.CreateFromConfig(cfg)
	if err != nil {
		t.Fatalf("CreateFromConfig error = %v", err)
	}
	if collector == nil {
		t.Fatal("expected non-nil collector")
	}
	if metrics.IsNoop(collector) {
		t.Error("expected real collector, not Noop")
	}
}

func TestFactory_CreateFromConfig_Noop(t *testing.T) {
	f := factory.New()

	cfg := &config.Metrics{
		Enable: true,
		Type:   config.MetricsTypeNoop,
	}

	collector, err := f.CreateFromConfig(cfg)
	if err != nil {
		t.Fatalf("CreateFromConfig error = %v", err)
	}
	// Note: MetricsTypeNoop creates a real collector without adapters, not Noop()
	if collector == nil {
		t.Fatal("expected non-nil collector")
	}
}

func TestFactory_CreateNoop(t *testing.T) {
	f := factory.New()

	collector := f.CreateNoop()
	if !metrics.IsNoop(collector) {
		t.Error("expected Noop collector")
	}
}
