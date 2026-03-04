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

func TestCollectorBuilder_Build_Disabled(t *testing.T) {
	// Nil config
	collector, err := factory.New(nil).Build()
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if !metrics.IsNoop(collector) {
		t.Error("expected Noop collector for nil config")
	}

	// Disabled config
	cfg := &config.Metrics{Enable: false}
	collector, err = factory.New(cfg).Build()
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if !metrics.IsNoop(collector) {
		t.Error("expected Noop collector for disabled config")
	}
}

func TestCollectorBuilder_Build_Prometheus(t *testing.T) {
	cfg := &config.Metrics{
		Enable:      true,
		Type:        config.MetricsTypePrometheus,
		ServiceName: "test",
	}

	collector, err := factory.New(cfg).Build()
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if collector == nil {
		t.Fatal("expected non-nil collector")
	}
	if metrics.IsNoop(collector) {
		t.Error("expected real collector, not Noop")
	}
}

func TestCollectorBuilder_Build_Noop(t *testing.T) {
	cfg := &config.Metrics{
		Enable: true,
		Type:   config.MetricsTypeNoop,
	}

	collector, err := factory.New(cfg).Build()
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	// Note: MetricsTypeNoop creates a real collector without adapters, not Noop()
	if collector == nil {
		t.Fatal("expected non-nil collector")
	}
}
