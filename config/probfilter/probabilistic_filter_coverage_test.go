// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package probfilterconfig_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	probfilterconfig "github.com/altessa-s/go-atlas/config/probfilter"
	storageconfig "github.com/altessa-s/go-atlas/config/storage"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

func fieldErrors(t *testing.T, err error) validation.Errors {
	t.Helper()
	verrs, ok := errors.AsType[validation.Errors](err)
	require.True(t, ok, "want validation.Errors, got %v", err)
	return verrs
}

func TestDefault(t *testing.T) {
	t.Parallel()

	c := probfilterconfig.Default()
	require.NotNil(t, c.Defaults.Bloom)
	require.NotNil(t, c.Defaults.Cuckoo)
	require.InDelta(t, 0.01, c.Defaults.Bloom.FalsePositiveRate, 1e-9)
	require.Equal(t, probfilterconfig.StorageTypeMemory, c.Defaults.Bloom.Storage.Type)
	require.NoError(t, c.Validate())
}

func TestBloomDefaults_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		mut   func(*probfilterconfig.BloomDefaults)
		field string
	}{
		{"unknown storage", func(c *probfilterconfig.BloomDefaults) { c.Storage.Type = "nats" }, "Storage"},
		{"zero fp rate", func(c *probfilterconfig.BloomDefaults) { c.FalsePositiveRate = 0 }, "FalsePositiveRate"},
		{"fp rate too high", func(c *probfilterconfig.BloomDefaults) { c.FalsePositiveRate = 0.9 }, "FalsePositiveRate"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := probfilterconfig.NewBloomDefaults()
			tc.mut(&c)
			require.Contains(t, fieldErrors(t, c.Validate()), tc.field)
		})
	}
}

func TestCuckooDefaults_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		mut   func(*probfilterconfig.CuckooDefaults)
		field string
	}{
		{"unknown storage", func(c *probfilterconfig.CuckooDefaults) { c.Storage.Type = "" }, "Storage"},
		{"multiplier too small", func(c *probfilterconfig.CuckooDefaults) { c.CapacityMultiplier = 1 }, "CapacityMultiplier"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := probfilterconfig.NewCuckooDefaults()
			tc.mut(&c)
			require.Contains(t, fieldErrors(t, c.Validate()), tc.field)
		})
	}
}

func TestDefaults_Validate(t *testing.T) {
	t.Parallel()

	require.NoError(t, (&probfilterconfig.Defaults{}).Validate(), "both sections are optional")

	d := probfilterconfig.NewDefaults()
	d.Bloom.FalsePositiveRate = 1
	d.Cuckoo.CapacityMultiplier = 0
	verrs := fieldErrors(t, d.Validate())
	require.Contains(t, verrs, "Bloom")
	require.Contains(t, verrs, "Cuckoo")
}

func TestStorage_Validate(t *testing.T) {
	t.Parallel()

	require.NoError(t, (&probfilterconfig.Storage{}).Validate(), "empty type inherits the default")
	require.NoError(t, (&probfilterconfig.Storage{Type: probfilterconfig.StorageTypeRedis}).Validate())
	require.NoError(t, (&probfilterconfig.Storage{
		Type: probfilterconfig.StorageTypeRedis, Redis: &storageconfig.RedisConfig{KeysPrefix: "bf:"},
	}).Validate())
	require.Contains(t, fieldErrors(t, (&probfilterconfig.Storage{Type: "nats"}).Validate()), "Type")
}

func TestBloomConfig_Validate(t *testing.T) {
	t.Parallel()

	require.NoError(t, (&probfilterconfig.BloomConfig{ExpectedItems: 1000, FalsePositiveRate: new(0.001)}).Validate())

	verrs := fieldErrors(t, (&probfilterconfig.BloomConfig{
		Storage:           &probfilterconfig.Storage{Type: "nats"},
		FalsePositiveRate: new(0.75),
	}).Validate())
	for _, f := range []string{"Storage", "ExpectedItems", "FalsePositiveRate"} {
		require.Contains(t, verrs, f)
	}
}

func TestCuckooConfig_Validate(t *testing.T) {
	t.Parallel()

	require.NoError(t, (&probfilterconfig.CuckooConfig{
		Capacity: 10, CapacityMultiplier: new(3.0),
	}).Validate())

	verrs := fieldErrors(t, (&probfilterconfig.CuckooConfig{
		Storage:            &probfilterconfig.Storage{Type: "nats"},
		CapacityMultiplier: new(20.0),
	}).Validate())
	for _, f := range []string{"Storage", "Capacity", "CapacityMultiplier"} {
		require.Contains(t, verrs, f)
	}
}

func TestFilter_Validate(t *testing.T) {
	t.Parallel()

	require.NoError(t, (&probfilterconfig.Filter{
		Type: probfilterconfig.TypeBloom, Bloom: &probfilterconfig.BloomConfig{ExpectedItems: 1},
	}).Validate())
	require.NoError(t, (&probfilterconfig.Filter{
		Type: probfilterconfig.TypeCuckoo, Cuckoo: &probfilterconfig.CuckooConfig{Capacity: 1},
	}).Validate())

	require.Contains(t, fieldErrors(t, (&probfilterconfig.Filter{}).Validate()), "Type")
	require.Contains(t, fieldErrors(t, (&probfilterconfig.Filter{Type: "quotient"}).Validate()), "Type")
	require.Contains(t, fieldErrors(t, (&probfilterconfig.Filter{Type: probfilterconfig.TypeBloom}).Validate()), "Bloom")
	require.Contains(t, fieldErrors(t, (&probfilterconfig.Filter{Type: probfilterconfig.TypeCuckoo}).Validate()), "Cuckoo")
}

func TestConfig_ValidateFilters(t *testing.T) {
	t.Parallel()

	c := probfilterconfig.Default()
	c.Filters = map[string]*probfilterconfig.Filter{
		"users": {Type: probfilterconfig.TypeBloom, Bloom: &probfilterconfig.BloomConfig{ExpectedItems: 100}},
	}
	require.NoError(t, c.Validate())

	c.Filters["broken"] = &probfilterconfig.Filter{Type: probfilterconfig.TypeCuckoo}
	err := c.Validate()
	require.Contains(t, fieldErrors(t, err), "Filters")
	require.ErrorContains(t, err, "broken")
}

// TestConfig_NilFilterEntry pins that an empty YAML entry (filters: {x: })
// is reported as a validation error instead of panicking on a nil *Filter.
func TestConfig_NilFilterEntry(t *testing.T) {
	t.Parallel()

	c := probfilterconfig.Default()
	c.Filters = map[string]*probfilterconfig.Filter{"x": nil}
	require.NotPanics(t, func() {
		require.Contains(t, fieldErrors(t, c.Validate()), "Filters")
	})
}
