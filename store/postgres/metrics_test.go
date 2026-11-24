// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/altessa-s/go-atlas/internal/testhelpers"
	"github.com/altessa-s/go-atlas/observability/metrics"
	"github.com/altessa-s/go-atlas/observability/metrics/adapters/memory"
)

func TestPoolMetrics(t *testing.T) {
	t.Parallel()
	adapter := memory.New()
	meter := metrics.New(metrics.WithAdapter(adapter), metrics.WithServiceName("test"))
	for _, pool := range []string{"foreground", "outbox"} {
		db, _ := testhelpers.NewFakeSQL(t, nil)
		db.SetMaxOpenConns(1)
		conn, err := db.Conn(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		stop := observePool(db, meter, pool)
		labels := metrics.Labels{"pool": pool}
		if adapter.GaugeValue("test_postgres_open_connections", labels) != 1 ||
			adapter.GaugeValue("test_postgres_in_use_connections", labels) != 1 {
			t.Errorf("%s: expected one open, in-use connection", pool)
		}
		wait, cancel := context.WithTimeout(t.Context(), 10*time.Millisecond)
		err = db.PingContext(wait)
		cancel()
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("waiting for %s: %v", pool, err)
		}
		if err = conn.Close(); err != nil {
			t.Error(err)
		}
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
		stop()
		if adapter.GaugeValue("test_postgres_idle_connections", labels) != 0 ||
			adapter.GaugeValue("test_postgres_in_use_connections", labels) != 0 ||
			adapter.CounterValue("test_postgres_wait_total", labels) != 1 ||
			adapter.CounterValue("test_postgres_wait_seconds_total", labels) != db.Stats().WaitDuration.Seconds() {
			t.Errorf("%s: metrics differ from pool stats %+v", pool, db.Stats())
		}
	}
}

func TestPoolMetricsAtCloseWithOutstandingLease(t *testing.T) {
	db, _ := testhelpers.NewFakeSQL(t, nil)
	conn, err := db.Conn(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	adapter := memory.New()
	meter := metrics.New(metrics.WithAdapter(adapter), metrics.WithServiceName("test"))
	stop := observePool(db, meter, DefaultPool)
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	stop()
	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}
	for _, gauge := range []string{"open_connections", "in_use_connections", "idle_connections"} {
		if value := adapter.GaugeValue("test_postgres_"+gauge, metrics.Labels{"pool": DefaultPool}); value != 0 {
			t.Errorf("%s after close = %v", gauge, value)
		}
	}
}
