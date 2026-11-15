// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package meilisearchconfig_test

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	meilisearchconfig "github.com/altessa-s/go-atlas/config/meilisearch"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

func TestDefault(t *testing.T) {
	t.Parallel()

	c := meilisearchconfig.Default()
	require.Equal(t, "http://localhost:7700", c.Host)
	require.Equal(t, 30*time.Second, c.Timeout)
	require.NoError(t, c.Validate())
}

func TestConfig_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		mut   func(*meilisearchconfig.Config)
		field string
	}{
		{"empty host", func(c *meilisearchconfig.Config) { c.Host = "" }, "Host"},
		{"invalid host", func(c *meilisearchconfig.Config) { c.Host = "::not-a-url" }, "Host"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := meilisearchconfig.Default()
			tc.mut(&c)
			verrs, ok := errors.AsType[validation.Errors](c.Validate())
			require.True(t, ok)
			require.Contains(t, verrs, tc.field)
		})
	}

	zeroTimeout := meilisearchconfig.Default()
	zeroTimeout.Timeout = 0
	require.NoError(t, zeroTimeout.Validate())
}
