// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package nodeconfig_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	nodeconfig "github.com/altessa-s/go-atlas/config/node"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

func TestConfig_Normalize(t *testing.T) {
	t.Parallel()

	c := nodeconfig.Config{Id: new("node-a_1")}
	c.Normalize()
	require.Equal(t, "NODE-A_1", *c.Id)

	empty := nodeconfig.Config{}
	empty.Normalize()
	require.Nil(t, empty.Id)
}

func TestConfig_Validate(t *testing.T) {
	t.Parallel()

	require.NoError(t, (&nodeconfig.Config{}).Validate())
	require.NoError(t, (&nodeconfig.Config{Id: new("Node_01-a")}).Validate())

	err := (&nodeconfig.Config{Id: new("node 01!")}).Validate()
	verrs, ok := errors.AsType[validation.Errors](err)
	require.True(t, ok, "want validation.Errors, got %v", err)
	require.Contains(t, verrs, "Id")
}
