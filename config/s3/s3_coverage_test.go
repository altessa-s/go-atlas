// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package s3config_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	s3config "github.com/altessa-s/go-atlas/config/s3"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

func TestDefault(t *testing.T) {
	t.Parallel()

	c := s3config.Default()
	require.Equal(t, "http://localhost:9000", c.Endpoint)

	err := c.Validate()
	verrs, ok := errors.AsType[validation.Errors](err)
	require.True(t, ok, "want validation.Errors, got %v", err)
	require.Contains(t, verrs, "AccessKey")
	require.Contains(t, verrs, "SecretKey")
	require.Contains(t, verrs, "Region")
	require.NotContains(t, verrs, "Endpoint")
}

func TestConfig_Validate(t *testing.T) {
	t.Parallel()

	valid := s3config.Default()
	valid.AccessKey = "ak"
	valid.SecretKey = "sk"
	valid.Region = "us-east-1"
	require.NoError(t, valid.Validate())

	bad := valid
	bad.Endpoint = "not a url"
	verrs, ok := errors.AsType[validation.Errors](bad.Validate())
	require.True(t, ok)
	require.Contains(t, verrs, "Endpoint")
}
