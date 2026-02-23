// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config

import (
	"github.com/altessa-s/go-atlas/core/runtime/appinfo"

	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// Default values for Tracing configuration.
const (
	defaultTracingEnable       = false
	defaultTracingType         = TracingTypeOTLP
	defaultTracingSamplerType  = SamplerTypeParentBased
	defaultTracingSamplerRatio = 1.0
)

// TracingType represents the type of tracing backend.
type TracingType string

// Supported tracing types.
const (
	// TracingTypeOTLP exports traces via OTLP protocol.
	TracingTypeOTLP TracingType = "otlp"

	// TracingTypeConsole outputs traces to console (for development).
	TracingTypeConsole TracingType = "console"

	// TracingTypeNoop disables tracing.
	TracingTypeNoop TracingType = "noop"
)

// SamplerType represents the type of trace sampler.
type SamplerType string

// Supported sampler types.
const (
	// SamplerTypeAlwaysOn samples all traces.
	SamplerTypeAlwaysOn SamplerType = "always_on"

	// SamplerTypeAlwaysOff samples no traces.
	SamplerTypeAlwaysOff SamplerType = "always_off"

	// SamplerTypeTraceIDRatio samples a fraction of traces based on trace ID.
	SamplerTypeTraceIDRatio SamplerType = "trace_id_ratio"

	// SamplerTypeParentBased defers to parent span's sampling decision.
	SamplerTypeParentBased SamplerType = "parent_based"
)

// OTLPProtocol represents the OTLP transport protocol.
type OTLPProtocol string

// Supported OTLP protocols.
const (
	// OTLPProtocolGRPC uses gRPC transport.
	OTLPProtocolGRPC OTLPProtocol = "grpc"

	// OTLPProtocolHTTP uses HTTP transport.
	OTLPProtocolHTTP OTLPProtocol = "http"
)

// Tracing represents the configuration for the tracing system.
//
// Example:
//
//	tracing:
//	  enable: true
//	  type: otlp
//	  serviceName: myapp
//	  serviceVersion: 1.0.0
//	  sampler:
//	    type: parent_based
//	    ratio: 0.1
//	  adapters:
//	    otlp:
//	      endpoint: localhost:4317
//	      insecure: true
type Tracing struct {
	// Enable determines whether tracing is enabled.
	// Defaults to false.
	Enable bool `yaml:"enable" default:"false"`

	// Type specifies the tracing backend type.
	// Valid values: "otlp", "console", "noop".
	// Defaults to "otlp".
	Type TracingType `yaml:"type" default:"otlp"`

	// ServiceName identifies the service in traces.
	// This is a required field when tracing is enabled.
	ServiceName string `yaml:"serviceName"`

	// ServiceVersion is the version of the service.
	// Optional but recommended for identifying deployments.
	ServiceVersion string `yaml:"serviceVersion"`

	// Environment identifies the deployment environment (e.g., "production", "staging").
	// Optional but recommended for filtering traces.
	Environment string `yaml:"environment"`

	// Sampler contains sampling configuration.
	// Controls which traces are recorded and exported.
	Sampler *TracingSampler `yaml:"sampler,omitempty" default:"-"`

	// Adapters contains adapter-specific configurations.
	Adapters *TracingAdapters `yaml:"adapters,omitempty" default:"-"`
}

// TracingAdapters contains configurations for different tracing adapters.
type TracingAdapters struct {
	// OTLP contains OTLP-specific configuration.
	// Only used when Type is "otlp".
	OTLP *TracingOTLP `yaml:"otlp,omitempty" default:"-"`

	// Console contains console-specific configuration.
	// Only used when Type is "console".
	Console *TracingConsole `yaml:"console,omitempty" default:"-"`
}

// DefaultTracing returns a Tracing configuration with default values.
func DefaultTracing() Tracing {
	sampler := DefaultTracingSampler()
	adapters := DefaultTracingAdapters()
	return Tracing{
		Enable:         defaultTracingEnable,
		Type:           defaultTracingType,
		Sampler:        &sampler,
		Adapters:       &adapters,
		ServiceName:    appinfo.Name,
		ServiceVersion: appinfo.Version,
		Environment:    appinfo.EnvLabel(),
	}
}

// DefaultTracingAdapters returns a TracingAdapters with default values.
func DefaultTracingAdapters() TracingAdapters {
	otlp := DefaultTracingOTLP()
	return TracingAdapters{
		OTLP: &otlp,
	}
}

// Validate performs validation on the Tracing configuration.
func (t *Tracing) Validate() error {
	if !t.Enable {
		return nil
	}

	return ValidateStruct(t,
		validation.Field(&t.Type,
			validation.Required,
			validation.In(
				TracingTypeOTLP,
				TracingTypeConsole,
				TracingTypeNoop,
			).Error("must be one of: otlp, console, noop"),
		),
		validation.Field(&t.ServiceName,
			validation.Required.Error("serviceName is required when tracing is enabled"),
		),
		validation.Field(&t.Sampler),
		validation.Field(&t.Adapters,
			validation.Required.When(t.Type == TracingTypeOTLP || t.Type == TracingTypeConsole),
		),
	)
}

// Validate performs validation on the TracingAdapters configuration.
func (a *TracingAdapters) Validate() error {
	return ValidateStruct(a,
		validation.Field(&a.OTLP),
		validation.Field(&a.Console),
	)
}

// TracingSampler contains sampling configuration.
type TracingSampler struct {
	// Type specifies the sampler type.
	// Valid values: "always_on", "always_off", "trace_id_ratio", "parent_based".
	// Defaults to "parent_based".
	Type SamplerType `yaml:"type" default:"parent_based"`

	// Ratio specifies the sampling ratio for trace_id_ratio and parent_based samplers.
	// Must be between 0.0 and 1.0.
	// 1.0 = sample all, 0.0 = sample none, 0.1 = sample 10%.
	// Defaults to 1.0.
	Ratio float64 `yaml:"ratio" default:"1.0"`
}

// DefaultTracingSampler returns a TracingSampler with default values.
func DefaultTracingSampler() TracingSampler {
	return TracingSampler{
		Type:  defaultTracingSamplerType,
		Ratio: defaultTracingSamplerRatio,
	}
}

// Validate performs validation on the TracingSampler configuration.
func (s *TracingSampler) Validate() error {
	return ValidateStruct(s,
		validation.Field(&s.Type,
			validation.Required,
			validation.In(
				SamplerTypeAlwaysOn,
				SamplerTypeAlwaysOff,
				SamplerTypeTraceIDRatio,
				SamplerTypeParentBased,
			).Error("must be one of: always_on, always_off, trace_id_ratio, parent_based"),
		),
		validation.Field(&s.Ratio,
			validation.Min(0.0).Error("must be >= 0.0"),
			validation.Max(1.0).Error("must be <= 1.0"),
		),
	)
}

// TracingOTLP contains OTLP-specific configuration.
type TracingOTLP struct {
	// Endpoint is the OTLP collector endpoint.
	// For gRPC, this is typically "host:4317".
	// For HTTP, this is typically "host:4318".
	// Defaults to "localhost:4317".
	Endpoint string `yaml:"endpoint" default:"localhost:4317"`

	// Protocol specifies the transport protocol.
	// Valid values: "grpc", "http".
	// Defaults to "grpc".
	Protocol OTLPProtocol `yaml:"protocol" default:"grpc"`

	// Insecure disables TLS.
	// Use for development or when connecting to a local collector.
	// Defaults to false.
	Insecure bool `yaml:"insecure" default:"false"`

	// Headers are custom headers to send with requests.
	// Useful for authentication (e.g., API keys).
	Headers map[string]string `yaml:"headers,omitempty"`

	// Compression enables gzip compression.
	// Defaults to true.
	Compression bool `yaml:"compression" default:"true"`
}

// DefaultTracingOTLP returns a TracingOTLP with default values.
func DefaultTracingOTLP() TracingOTLP {
	return TracingOTLP{
		Endpoint:    "localhost:4317",
		Protocol:    OTLPProtocolGRPC,
		Insecure:    false,
		Compression: true,
	}
}

// Validate performs validation on the TracingOTLP configuration.
func (o *TracingOTLP) Validate() error {
	return ValidateStruct(o,
		validation.Field(&o.Endpoint, validation.Required),
		validation.Field(&o.Protocol,
			validation.Required,
			validation.In(
				OTLPProtocolGRPC,
				OTLPProtocolHTTP,
			).Error("must be one of: grpc, http"),
		),
	)
}

// TracingConsole contains console output configuration.
type TracingConsole struct {
	// PrettyPrint enables indented JSON output.
	// Defaults to true.
	PrettyPrint bool `yaml:"prettyPrint" default:"true"`

	// Timestamps includes timestamps in output.
	// Defaults to true.
	Timestamps bool `yaml:"timestamps" default:"true"`
}

// DefaultTracingConsole returns a TracingConsole with default values.
func DefaultTracingConsole() TracingConsole {
	return TracingConsole{
		PrettyPrint: true,
		Timestamps:  true,
	}
}

// Validate performs validation on the TracingConsole configuration.
func (c *TracingConsole) Validate() error {
	return nil
}
