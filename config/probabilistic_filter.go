// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config

import (
	ozzo_rules "github.com/altessa-s/ozzo-rules"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// ProbabilisticFilterType defines the type of probabilistic filter to use.
type ProbabilisticFilterType string

const (
	// ProbabilisticFilterTypeBloom represents a Bloom filter.
	ProbabilisticFilterTypeBloom ProbabilisticFilterType = "bloom"

	// ProbabilisticFilterTypeCuckoo represents a Cuckoo filter.
	ProbabilisticFilterTypeCuckoo ProbabilisticFilterType = "cuckoo"
)

// ProbabilisticFilterStorageType defines the storage backend for probabilistic filters.
type ProbabilisticFilterStorageType string

const (
	// ProbabilisticFilterStorageTypeMemory represents in-memory storage.
	ProbabilisticFilterStorageTypeMemory ProbabilisticFilterStorageType = "memory"

	// ProbabilisticFilterStorageTypeRedis represents Redis storage.
	ProbabilisticFilterStorageTypeRedis ProbabilisticFilterStorageType = "redis"
)

var probabilisticFilterStorageAllowedTypesAny = []any{
	ProbabilisticFilterStorageTypeMemory,
	ProbabilisticFilterStorageTypeRedis,
}

// Validation constants for probabilistic filters.
const (
	// minFalsePositiveRate is the minimum allowed false positive rate.
	minFalsePositiveRate = 0.0001
	// maxFalsePositiveRate is the maximum allowed false positive rate.
	maxFalsePositiveRate = 0.5
	// minCapacityMultiplier is the minimum allowed capacity multiplier.
	minCapacityMultiplier = 1.1
	// maxCapacityMultiplier is the maximum allowed capacity multiplier.
	maxCapacityMultiplier = 10.0
	// minMaxCapacity is the minimum allowed max capacity.
	minMaxCapacity = 1000
	// fingerprintSize8 is a valid fingerprint size (8 bits).
	fingerprintSize8 = 8
	// fingerprintSize12 is a valid fingerprint size (12 bits).
	fingerprintSize12 = 12
	// fingerprintSize16 is a valid fingerprint size (16 bits).
	fingerprintSize16 = 16
)

// Default values for ProbabilisticFilter configuration.
const (
	defaultProbFilterStorage               = ProbabilisticFilterStorageTypeMemory
	defaultProbFilterBloomFPRate           = 0.01
	defaultProbFilterBloomRebuildCron      = "0 0 * * * *"
	defaultProbFilterBloomRebuildOnStart   = true
	defaultProbFilterCuckooFingerprintSize = 12
	defaultProbFilterCuckooCapacityMult    = 2.0
	defaultProbFilterCuckooMaxCapacity     = int64(100000000)
)

// ProbabilisticFilterBloomDefaults defines default settings for Bloom filters.
// These values are used when not overridden in specific filter configuration.
type ProbabilisticFilterBloomDefaults struct {
	// Storage defines the storage backend for the filter.
	Storage ProbabilisticFilterStorageType `yaml:"storage" default:"memory"`

	// FalsePositiveRate is the target false positive rate (0.01 = 1%).
	FalsePositiveRate float64 `yaml:"falsePositiveRate" default:"0.01"`

	// RebuildCron is the cron expression for periodic filter rebuild.
	RebuildCron string `yaml:"rebuildCron" default:"0 0 * * * *"`

	// RebuildOnStart enables rebuilding filter on service startup.
	RebuildOnStart bool `yaml:"rebuildOnStart" default:"true"`
}

// DefaultProbabilisticFilterBloomDefaults returns a ProbabilisticFilterBloomDefaults with default values.
func DefaultProbabilisticFilterBloomDefaults() ProbabilisticFilterBloomDefaults {
	return ProbabilisticFilterBloomDefaults{
		Storage:           defaultProbFilterStorage,
		FalsePositiveRate: defaultProbFilterBloomFPRate,
		RebuildCron:       defaultProbFilterBloomRebuildCron,
		RebuildOnStart:    defaultProbFilterBloomRebuildOnStart,
	}
}

// Validate performs validation of the Bloom defaults configuration.
func (c *ProbabilisticFilterBloomDefaults) Validate() error {
	return ValidateStruct(c,
		validation.Field(&c.Storage, ozzo_rules.OneOf(probabilisticFilterStorageAllowedTypesAny...)),
		validation.Field(&c.FalsePositiveRate, validation.Min(minFalsePositiveRate), validation.Max(maxFalsePositiveRate)),
	)
}

// ProbabilisticFilterCuckooDefaults defines default settings for Cuckoo filters.
// These values are used when not overridden in specific filter configuration.
type ProbabilisticFilterCuckooDefaults struct {
	// Storage defines the storage backend for the filter.
	Storage ProbabilisticFilterStorageType `yaml:"storage" default:"memory"`

	// FingerprintSize is the fingerprint size in bits (8, 12, or 16).
	FingerprintSize int `yaml:"fingerprintSize" default:"12"`

	// CapacityMultiplier is the multiplier for auto-rebuild on overflow.
	CapacityMultiplier float64 `yaml:"capacityMultiplier" default:"2.0"`

	// MaxCapacity is the maximum capacity limit.
	MaxCapacity int64 `yaml:"maxCapacity" default:"100000000"`
}

// DefaultProbabilisticFilterCuckooDefaults returns a ProbabilisticFilterCuckooDefaults with default values.
func DefaultProbabilisticFilterCuckooDefaults() ProbabilisticFilterCuckooDefaults {
	return ProbabilisticFilterCuckooDefaults{
		Storage:            defaultProbFilterStorage,
		FingerprintSize:    defaultProbFilterCuckooFingerprintSize,
		CapacityMultiplier: defaultProbFilterCuckooCapacityMult,
		MaxCapacity:        defaultProbFilterCuckooMaxCapacity,
	}
}

// Validate performs validation of the Cuckoo defaults configuration.
func (c *ProbabilisticFilterCuckooDefaults) Validate() error {
	return ValidateStruct(c,
		validation.Field(&c.Storage, ozzo_rules.OneOf(probabilisticFilterStorageAllowedTypesAny...)),
		validation.Field(&c.FingerprintSize, validation.In(fingerprintSize8, fingerprintSize12, fingerprintSize16)),
		validation.Field(&c.CapacityMultiplier, validation.Min(minCapacityMultiplier), validation.Max(maxCapacityMultiplier)),
		validation.Field(&c.MaxCapacity, validation.Min(minMaxCapacity)),
	)
}

// ProbabilisticFilterDefaults defines default settings for all filters.
type ProbabilisticFilterDefaults struct {
	// Bloom contains default Bloom filter settings.
	Bloom *ProbabilisticFilterBloomDefaults `yaml:"bloom"`

	// Cuckoo contains default Cuckoo filter settings.
	Cuckoo *ProbabilisticFilterCuckooDefaults `yaml:"cuckoo"`
}

// DefaultProbabilisticFilterDefaults returns a ProbabilisticFilterDefaults with default values.
func DefaultProbabilisticFilterDefaults() ProbabilisticFilterDefaults {
	bloom := DefaultProbabilisticFilterBloomDefaults()
	cuckoo := DefaultProbabilisticFilterCuckooDefaults()
	return ProbabilisticFilterDefaults{
		Bloom:  &bloom,
		Cuckoo: &cuckoo,
	}
}

// Validate performs validation of the filter defaults configuration.
func (c *ProbabilisticFilterDefaults) Validate() error {
	return ValidateStruct(c,
		validation.Field(&c.Bloom),
		validation.Field(&c.Cuckoo),
	)
}

// ProbabilisticFilterBloomConfig defines configuration for a specific Bloom filter.
type ProbabilisticFilterBloomConfig struct {
	// Storage defines the storage backend for the filter.
	Storage *ProbabilisticFilterStorageType `yaml:"storage,omitempty"`

	// ExpectedItems is the expected number of elements in the filter.
	ExpectedItems int64 `yaml:"expectedItems"`

	// FalsePositiveRate is the target false positive rate.
	FalsePositiveRate *float64 `yaml:"falsePositiveRate,omitempty"`

	// RebuildCron is the cron expression for periodic filter rebuild.
	RebuildCron *string `yaml:"rebuildCron,omitempty"`

	// RebuildOnStart enables rebuilding filter on service startup.
	RebuildOnStart *bool `yaml:"rebuildOnStart,omitempty"`

	// Redis defines Redis-specific configuration.
	Redis *StorageRedisConfig `yaml:"redis,omitempty" default:"-"`
}

// Validate performs validation of the Bloom filter configuration.
func (c *ProbabilisticFilterBloomConfig) Validate() error {
	return ValidateStruct(c,
		validation.Field(&c.Storage, validation.When(c.Storage != nil, ozzo_rules.OneOf(probabilisticFilterStorageAllowedTypesAny...))),
		validation.Field(&c.ExpectedItems, validation.Required, validation.Min(1)),
		validation.Field(&c.FalsePositiveRate,
			validation.When(c.FalsePositiveRate != nil, validation.Min(minFalsePositiveRate), validation.Max(maxFalsePositiveRate))),
		validation.Field(&c.Redis,
			validation.When(c.Storage != nil && *c.Storage == ProbabilisticFilterStorageTypeRedis, validation.NilOrNotEmpty)),
	)
}

// ProbabilisticFilterCuckooConfig defines configuration for a specific Cuckoo filter.
type ProbabilisticFilterCuckooConfig struct {
	// Storage defines the storage backend for the filter.
	Storage *ProbabilisticFilterStorageType `yaml:"storage,omitempty"`

	// Capacity is the initial capacity of the filter.
	Capacity int64 `yaml:"capacity"`

	// FingerprintSize is the fingerprint size in bits.
	FingerprintSize *int `yaml:"fingerprintSize,omitempty"`

	// CapacityMultiplier is the multiplier for auto-rebuild on overflow.
	CapacityMultiplier *float64 `yaml:"capacityMultiplier,omitempty"`

	// MaxCapacity is the maximum capacity limit.
	MaxCapacity *int64 `yaml:"maxCapacity,omitempty"`

	// Redis defines Redis-specific configuration.
	Redis *StorageRedisConfig `yaml:"redis,omitempty" default:"-"`
}

// Validate performs validation of the Cuckoo filter configuration.
func (c *ProbabilisticFilterCuckooConfig) Validate() error {
	return ValidateStruct(c,
		validation.Field(&c.Storage, validation.When(c.Storage != nil, ozzo_rules.OneOf(probabilisticFilterStorageAllowedTypesAny...))),
		validation.Field(&c.Capacity, validation.Required, validation.Min(1)),
		validation.Field(&c.FingerprintSize,
			validation.When(c.FingerprintSize != nil, validation.In(fingerprintSize8, fingerprintSize12, fingerprintSize16))),
		validation.Field(&c.CapacityMultiplier,
			validation.When(c.CapacityMultiplier != nil, validation.Min(minCapacityMultiplier), validation.Max(maxCapacityMultiplier))),
		validation.Field(&c.MaxCapacity,
			validation.When(c.MaxCapacity != nil, validation.Min(minMaxCapacity))),
		validation.Field(&c.Redis,
			validation.When(c.Storage != nil && *c.Storage == ProbabilisticFilterStorageTypeRedis, validation.NilOrNotEmpty)),
	)
}

// ProbabilisticFilterConfig defines configuration for a single named filter.
type ProbabilisticFilterConfig struct {
	// Type defines the type of probabilistic filter to use.
	Type ProbabilisticFilterType `yaml:"type" default:"bloom"`

	// Bloom defines Bloom filter-specific configuration.
	Bloom *ProbabilisticFilterBloomConfig `yaml:"bloom,omitempty" default:"-"`

	// Cuckoo defines Cuckoo filter-specific configuration.
	Cuckoo *ProbabilisticFilterCuckooConfig `yaml:"cuckoo,omitempty" default:"-"`
}

var probabilisticFilterAllowedTypes = []ProbabilisticFilterType{
	ProbabilisticFilterTypeBloom,
	ProbabilisticFilterTypeCuckoo,
}

// Validate performs validation of the filter configuration.
func (c *ProbabilisticFilterConfig) Validate() error {
	allowedTypes := make([]any, 0, len(probabilisticFilterAllowedTypes))
	for _, v := range probabilisticFilterAllowedTypes {
		allowedTypes = append(allowedTypes, v)
	}

	return ValidateStruct(c,
		validation.Field(&c.Type, validation.Required, ozzo_rules.OneOf(allowedTypes...)),
		validation.Field(&c.Bloom,
			validation.When(c.Type == ProbabilisticFilterTypeBloom, validation.Required)),
		validation.Field(&c.Cuckoo,
			validation.When(c.Type == ProbabilisticFilterTypeCuckoo, validation.Required)),
	)
}

// ProbabilisticFilter defines the configuration for probabilistic filters.
// Supports multiple named filters with shared defaults for existence checks optimization.
type ProbabilisticFilter struct {
	// Defaults contains default settings for all filters.
	Defaults *ProbabilisticFilterDefaults `yaml:"defaults"`

	// Filters contains named filter configurations.
	Filters map[string]*ProbabilisticFilterConfig `yaml:"filters"`
}

// DefaultProbabilisticFilter returns a ProbabilisticFilter configuration with default values.
func DefaultProbabilisticFilter() ProbabilisticFilter {
	defaults := DefaultProbabilisticFilterDefaults()
	return ProbabilisticFilter{
		Defaults: &defaults,
	}
}

// Validate performs validation of the probabilistic filter configuration.
// Returns an error if validation fails, nil otherwise.
func (p *ProbabilisticFilter) Validate() error {
	return ValidateStruct(p,
		validation.Field(&p.Defaults),
		validation.Field(&p.Filters, validation.By(validateProbabilisticFilters)),
	)
}

func validateProbabilisticFilters(value any) error {
	filters, ok := value.(map[string]*ProbabilisticFilterConfig)
	if !ok {
		return nil
	}

	for name, cfg := range filters {
		if cfg == nil {
			continue
		}
		if err := cfg.Validate(); err != nil {
			return validation.NewError("validation_filter_invalid",
				"filter '"+name+"': "+err.Error())
		}
	}
	return nil
}
