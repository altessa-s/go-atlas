// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config

import "time"

// DispatchConfig defines the configuration for an async dispatch engine.
// Controls how items are buffered, batched, retried, and optionally
// persisted to a write-ahead log.
//
// Example:
//
//	dispatch := &config.DispatchConfig{
//		BufferSize:    20000,
//		BatchSize:     200,
//		FlushInterval: 500 * time.Millisecond,
//		Workers:       4,
//		WAL: WALConfig{Enabled: true, Dir: "./var/dispatch/wal"},
//	}
type DispatchConfig struct {
	// BufferSize is the in-memory queue capacity.
	BufferSize int `yaml:"bufferSize" default:"10000"`

	// BatchSize is the maximum batch size passed to Sink.StoreBatch.
	BatchSize int `yaml:"batchSize" default:"100"`

	// FlushInterval is the maximum wait before flushing a partial batch.
	FlushInterval time.Duration `yaml:"flushInterval" default:"1s"`

	// Workers is the number of concurrent dispatch goroutines.
	Workers int `yaml:"workers" default:"2"`

	// RetryAttempts is the maximum retries per failed batch.
	RetryAttempts int `yaml:"retryAttempts" default:"3"`

	// RetryBackoff is the base duration for exponential backoff.
	RetryBackoff time.Duration `yaml:"retryBackoff" default:"100ms"`

	// BackPressure enables back-pressure mode where Submit blocks when
	// the buffer is full instead of dropping items.
	BackPressure bool `yaml:"backPressure"`

	// MetricsSubsystem is the Prometheus subsystem name for engine metrics.
	MetricsSubsystem string `yaml:"metricsSubsystem" default:"async"`

	// WAL configures the optional crash-safe write-ahead log.
	WAL WALConfig `yaml:"wal"`
}
