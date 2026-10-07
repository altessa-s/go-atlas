// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package probfilterconfig

import (
	storageconfig "github.com/altessa-s/go-atlas/config/storage"
	validationconfig "github.com/altessa-s/go-atlas/config/validation"
	ozzo_rules "github.com/altessa-s/ozzo-rules"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// Type defines the type of probabilistic filter to use.
type Type string

const (
	// TypeBloom represents a Bloom filter.
	TypeBloom Type = "bloom"

	// TypeCuckoo represents a Cuckoo filter.
	TypeCuckoo Type = "cuckoo"
)

// StorageType defines the storage backend for probabilistic filters.
type StorageType string

const (
	// StorageTypeMemory represents in-memory storage.
	StorageTypeMemory StorageType = "memory"

	// StorageTypeRedis represents Redis storage.
	StorageTypeRedis StorageType = "redis"
)

var probabilisticFilterStorageAllowedTypesAny = []any{
	StorageTypeMemory,
	StorageTypeRedis,
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

// Default values for Config configuration.
const (
	defaultProbFilterStorage               = StorageTypeMemory
	defaultProbFilterBloomFPRate           = 0.01
	defaultProbFilterBloomRebuildCron      = "0 0 * * * *"
	defaultProbFilterBloomRebuildOnStart   = true
	defaultProbFilterCuckooFingerprintSize = 12
	defaultProbFilterCuckooCapacityMult    = 2.0
	defaultProbFilterCuckooMaxCapacity     = int64(100000000)
)

// Storage selects where a filter keeps its data.
type Storage struct {
	// Type is the storage backend: memory or redis. Empty inherits the default.
	Type StorageType `yaml:"type"`

	// Redis defines Redis-specific configuration; used when Type is redis.
	Redis *storageconfig.RedisConfig `yaml:"redis" default:"-"`
}

// Validate performs validation of the filter storage configuration. An empty
// Type inherits the default storage, so a filter may override only its Redis
// settings.
func (c *Storage) Validate() error {
	return validationconfig.ValidateStruct(c,
		validation.Field(&c.Type, validation.When(c.Type != "", ozzo_rules.OneOf(probabilisticFilterStorageAllowedTypesAny...))),
		validation.Field(&c.Redis, validation.When(c.Type == StorageTypeRedis, validation.NilOrNotEmpty)),
	)
}

// DefaultStorage selects the storage backend of filters that set none.
type DefaultStorage struct {
	// Type is the storage backend: memory or redis.
	Type StorageType `yaml:"type" default:"memory"`
}

// Validate performs validation of the default storage configuration.
func (c *DefaultStorage) Validate() error {
	return validationconfig.ValidateStruct(c,
		validation.Field(&c.Type, ozzo_rules.OneOf(probabilisticFilterStorageAllowedTypesAny...)),
	)
}

// BloomDefaults defines default settings for Bloom filters.
// These values are used when not overridden in specific filter configuration.
type BloomDefaults struct {
	// Storage defines the storage backend filters use unless they set their own.
	Storage DefaultStorage `yaml:"storage"`

	// FalsePositiveRate is the target false positive rate (0.01 = 1%).
	FalsePositiveRate float64 `yaml:"falsePositiveRate" default:"0.01"`

	// RebuildCron is the cron expression for periodic filter rebuild. The
	// probfilter factory schedules it only when both a data loader and a
	// scheduler are injected; an empty value disables periodic rebuilds.
	RebuildCron string `yaml:"rebuildCron" default:"0 0 * * * *"`

	// RebuildOnStart enables rebuilding filter on service startup. The
	// probfilter factory honors it only when a data loader is injected.
	RebuildOnStart bool `yaml:"rebuildOnStart" default:"true"`
}

// NewBloomDefaults returns a BloomDefaults with default values.
func NewBloomDefaults() BloomDefaults {
	return BloomDefaults{
		Storage:           DefaultStorage{Type: defaultProbFilterStorage},
		FalsePositiveRate: defaultProbFilterBloomFPRate,
		RebuildCron:       defaultProbFilterBloomRebuildCron,
		RebuildOnStart:    defaultProbFilterBloomRebuildOnStart,
	}
}

// Validate performs validation of the Bloom defaults configuration.
func (c *BloomDefaults) Validate() error {
	return validationconfig.ValidateStruct(c,
		validationconfig.NestedField(&c.Storage),
		// Required is paired with Min because ozzo-validation skips every
		// rule but Required for a zero value — Min alone accepts 0.
		validation.Field(&c.FalsePositiveRate, validation.Required, validation.Min(minFalsePositiveRate), validation.Max(maxFalsePositiveRate)),
	)
}

// CuckooDefaults defines default settings for Cuckoo filters.
// These values are used when not overridden in specific filter configuration.
type CuckooDefaults struct {
	// Storage defines the storage backend filters use unless they set their own.
	Storage DefaultStorage `yaml:"storage"`

	// FingerprintSize is the fingerprint size in bits (8, 12, or 16).
	//
	// Deprecated: ignored. Both storage backends use fixed 8-bit fingerprints
	// (the in-memory filter and RedisBloom CF.* in Redis). The field is
	// still validated so existing configurations keep loading.
	FingerprintSize int `yaml:"fingerprintSize" default:"12"`

	// CapacityMultiplier is the growth factor applied when the filter fills
	// up. Only the Redis storage honors it, as the RedisBloom EXPANSION
	// argument (rounded up to an integer); the memory storage cannot grow.
	CapacityMultiplier float64 `yaml:"capacityMultiplier" default:"2.0"`

	// MaxCapacity is the maximum capacity limit.
	//
	// Deprecated: ignored. Neither storage backend can bound filter growth.
	// The field is still validated so existing configurations keep loading.
	MaxCapacity int64 `yaml:"maxCapacity" default:"100000000"`
}

// NewCuckooDefaults returns a CuckooDefaults with default values.
func NewCuckooDefaults() CuckooDefaults {
	return CuckooDefaults{
		Storage:            DefaultStorage{Type: defaultProbFilterStorage},
		FingerprintSize:    defaultProbFilterCuckooFingerprintSize,
		CapacityMultiplier: defaultProbFilterCuckooCapacityMult,
		MaxCapacity:        defaultProbFilterCuckooMaxCapacity,
	}
}

// Validate performs validation of the Cuckoo defaults configuration.
func (c *CuckooDefaults) Validate() error {
	return validationconfig.ValidateStruct(c,
		validationconfig.NestedField(&c.Storage),
		validation.Field(&c.FingerprintSize, validation.In(fingerprintSize8, fingerprintSize12, fingerprintSize16)),
		// Required is paired with Min because ozzo-validation skips every
		// rule but Required for a zero value — Min alone accepts 0.
		validation.Field(&c.CapacityMultiplier, validation.Required, validation.Min(minCapacityMultiplier), validation.Max(maxCapacityMultiplier)),
		validation.Field(&c.MaxCapacity, validation.Required, validation.Min(minMaxCapacity)),
	)
}

// Defaults defines default settings for all filters.
type Defaults struct {
	// Bloom contains default Bloom filter settings.
	Bloom *BloomDefaults `yaml:"bloom"`

	// Cuckoo contains default Cuckoo filter settings.
	Cuckoo *CuckooDefaults `yaml:"cuckoo"`
}

// NewDefaults returns a Defaults with default values.
func NewDefaults() Defaults {
	bloom := NewBloomDefaults()
	cuckoo := NewCuckooDefaults()
	return Defaults{
		Bloom:  &bloom,
		Cuckoo: &cuckoo,
	}
}

// Validate performs validation of the filter defaults configuration.
func (c *Defaults) Validate() error {
	return validationconfig.ValidateStruct(c,
		validation.Field(&c.Bloom),
		validation.Field(&c.Cuckoo),
	)
}

// BloomConfig defines configuration for a specific Bloom filter.
type BloomConfig struct {
	// Storage overrides the storage backend of the defaults; nil inherits them.
	Storage *Storage `yaml:"storage" default:"-"`

	// ExpectedItems is the expected number of elements in the filter.
	ExpectedItems int64 `yaml:"expectedItems"`

	// FalsePositiveRate is the target false positive rate.
	FalsePositiveRate *float64 `yaml:"falsePositiveRate"`

	// RebuildCron is the cron expression for periodic filter rebuild.
	RebuildCron *string `yaml:"rebuildCron"`

	// RebuildOnStart enables rebuilding filter on service startup.
	RebuildOnStart *bool `yaml:"rebuildOnStart"`
}

// Validate performs validation of the Bloom filter configuration.
func (c *BloomConfig) Validate() error {
	return validationconfig.ValidateStruct(c,
		validation.Field(&c.Storage),
		validation.Field(&c.ExpectedItems, validation.Required, validation.Min(1)),
		// Required is paired with Min because ozzo-validation skips every rule
		// but Required for a zero value — an explicit `falsePositiveRate: 0`
		// is a non-nil pointer to zero, which Min alone accepts.
		validation.Field(&c.FalsePositiveRate,
			validation.When(c.FalsePositiveRate != nil,
				validation.Required, validation.Min(minFalsePositiveRate), validation.Max(maxFalsePositiveRate))),
	)
}

// CuckooConfig defines configuration for a specific Cuckoo filter.
type CuckooConfig struct {
	// Storage overrides the storage backend of the defaults; nil inherits them.
	Storage *Storage `yaml:"storage" default:"-"`

	// Capacity is the initial capacity of the filter.
	Capacity int64 `yaml:"capacity"`

	// FingerprintSize is the fingerprint size in bits.
	//
	// Deprecated: ignored; see [CuckooDefaults.FingerprintSize].
	FingerprintSize *int `yaml:"fingerprintSize"`

	// CapacityMultiplier is the growth factor applied when the filter fills
	// up; Redis storage only, see [CuckooDefaults.CapacityMultiplier].
	CapacityMultiplier *float64 `yaml:"capacityMultiplier"`

	// MaxCapacity is the maximum capacity limit.
	//
	// Deprecated: ignored; see [CuckooDefaults.MaxCapacity].
	MaxCapacity *int64 `yaml:"maxCapacity"`
}

// Validate performs validation of the Cuckoo filter configuration.
func (c *CuckooConfig) Validate() error {
	return validationconfig.ValidateStruct(c,
		validation.Field(&c.Storage),
		validation.Field(&c.Capacity, validation.Required, validation.Min(1)),
		validation.Field(&c.FingerprintSize,
			validation.When(c.FingerprintSize != nil, validation.In(fingerprintSize8, fingerprintSize12, fingerprintSize16))),
		// Required is paired with Min because ozzo-validation skips every rule
		// but Required for a zero value — an explicit `capacityMultiplier: 0`
		// is a non-nil pointer to zero, which Min alone accepts.
		validation.Field(&c.CapacityMultiplier,
			validation.When(c.CapacityMultiplier != nil,
				validation.Required, validation.Min(minCapacityMultiplier), validation.Max(maxCapacityMultiplier))),
		validation.Field(&c.MaxCapacity,
			validation.When(c.MaxCapacity != nil, validation.Required, validation.Min(minMaxCapacity))),
	)
}

// Filter defines configuration for a single named filter.
type Filter struct {
	// Type defines the type of probabilistic filter to use.
	Type Type `yaml:"type" default:"bloom"`

	// Bloom defines Bloom filter-specific configuration.
	Bloom *BloomConfig `yaml:"bloom" default:"-"`

	// Cuckoo defines Cuckoo filter-specific configuration.
	Cuckoo *CuckooConfig `yaml:"cuckoo" default:"-"`
}

var probabilisticFilterAllowedTypes = []Type{
	TypeBloom,
	TypeCuckoo,
}

// Validate performs validation of the filter configuration.
func (c *Filter) Validate() error {
	allowedTypes := make([]any, 0, len(probabilisticFilterAllowedTypes))
	for _, v := range probabilisticFilterAllowedTypes {
		allowedTypes = append(allowedTypes, v)
	}

	return validationconfig.ValidateStruct(c,
		validation.Field(&c.Type, validation.Required, ozzo_rules.OneOf(allowedTypes...)),
		validation.Field(&c.Bloom,
			validation.When(c.Type == TypeBloom, validation.Required)),
		validation.Field(&c.Cuckoo,
			validation.When(c.Type == TypeCuckoo, validation.Required)),
	)
}

// Config defines the configuration for probabilistic filters.
// Supports multiple named filters with shared defaults for existence checks optimization.
type Config struct {
	// Defaults contains default settings for all filters.
	Defaults *Defaults `yaml:"defaults"`

	// Filters contains named filter configurations.
	Filters map[string]*Filter `yaml:"filters"`

	// SkipEvictionPolicyCheck disables the startup check that the Redis server
	// of a Redis-backed filter has no allkeys-* maxmemory-policy, which could
	// evict the filter's keys. Defaults to false.
	SkipEvictionPolicyCheck bool `yaml:"skipEvictionPolicyCheck" default:"false"`
}

// Default returns a Config configuration with default values.
func Default() Config {
	defaults := NewDefaults()
	return Config{
		Defaults: &defaults,
	}
}

// Validate performs validation of the probabilistic filter configuration.
// Returns an error if validation fails, nil otherwise.
func (p *Config) Validate() error {
	return validationconfig.ValidateStruct(p,
		validation.Field(&p.Defaults),
		validation.Field(&p.Filters, validation.By(validateProbabilisticFilters)),
	)
}

func validateProbabilisticFilters(value any) error {
	filters, ok := value.(map[string]*Filter)
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
