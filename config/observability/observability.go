// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package observabilityconfig

import (
	validationconfig "github.com/altessa-s/go-atlas/config/validation"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// Config represents the configuration for observability features
// including metrics, tracing, and logging.
//
// Example:
//
//	observability:
//	  metrics:
//	    enable: true
//	    type: prometheus
//	    namespace: myapp
//	  tracing:
//	    enable: true
//	    type: otlp
//	    serviceName: myapp
//	    otlp:
//	      endpoint: localhost:4317
type Config struct {
	// Metrics contains configuration for the metrics collection system.
	Metrics *Metrics `yaml:"metrics" default:"-"`

	// Tracing contains configuration for distributed tracing.
	Tracing *Tracing `yaml:"tracing" default:"-"`
}

// Default returns an Observability configuration with default values.
func Default() Config {
	metrics := DefaultMetrics()
	tracing := DefaultTracing()
	return Config{
		Metrics: &metrics,
		Tracing: &tracing,
	}
}

// Validate performs validation on the Observability configuration.
func (o Config) Validate() error {
	return validationconfig.ValidateStruct(&o,
		validation.Field(&o.Metrics),
		validation.Field(&o.Tracing),
	)
}
