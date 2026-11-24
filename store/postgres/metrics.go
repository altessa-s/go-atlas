// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package postgres

import (
	"database/sql"
	"database/sql/driver"
	"io"
	"time"

	"github.com/altessa-s/go-atlas/observability/metrics"
)

var _ io.Closer = (*poolConnector)(nil)

// database/sql closes the connector after closing the pool.
type poolConnector struct {
	driver.Connector
	stop func()
}

func (c *poolConnector) Close() error {
	if c.stop != nil {
		c.stop()
	}
	return nil
}

func observePool(db *sql.DB, meter metrics.Meter, name string) func() {
	meter = meter.WithSubsystem("postgres")
	opts := func(name, help string) metrics.MetricOpts {
		return metrics.MetricOpts{Name: name, Help: help, LabelNames: []string{"pool"}}
	}
	labels := metrics.Labels{"pool": name}
	open := meter.Gauge(opts("open_connections", "Number of open PostgreSQL connections.")).WithLabels(labels)
	inUse := meter.Gauge(opts("in_use_connections", "Number of PostgreSQL connections in use.")).WithLabels(labels)
	idle := meter.Gauge(opts("idle_connections", "Number of idle PostgreSQL connections.")).WithLabels(labels)
	waits := meter.Counter(opts("wait_total", "Number of waits for a PostgreSQL connection.")).WithLabels(labels)
	waitTime := meter.Counter(opts("wait_seconds_total", "Time spent waiting for a PostgreSQL connection in seconds.")).WithLabels(labels)
	var previous sql.DBStats
	sample := func() {
		stats := db.Stats()
		open.Set(float64(stats.OpenConnections))
		inUse.Set(float64(stats.InUse))
		idle.Set(float64(stats.Idle))
		waits.Add(float64(stats.WaitCount - previous.WaitCount))
		waitTime.Add((stats.WaitDuration - previous.WaitDuration).Seconds())
		previous = stats
	}
	sample()
	stop, done := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				sample()
			case <-stop:
				sample()
				return
			}
		}
	}()
	return func() {
		close(stop)
		<-done
	}
}
