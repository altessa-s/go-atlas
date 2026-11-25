// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/transport/http/server/router/std"

	httpconfig "github.com/altessa-s/go-atlas/config/http"
	corefactory "github.com/altessa-s/go-atlas/factory/core"
)

func TestServerSettingsAddressOverride(t *testing.T) {
	t.Parallel()
	const override = "127.0.0.1:1234"
	base := corefactory.NewBase(nil)
	settings := serverSettings{cfg: &httpconfig.Config{ListenAddress: "localhost:8080"}, listenAddress: new(override)}
	rt := std.New()
	srv, err := settings.newServer(&base, rt, rt, nil)
	require.NoError(t, err)
	require.Equal(t, override, srv.Address())
}
