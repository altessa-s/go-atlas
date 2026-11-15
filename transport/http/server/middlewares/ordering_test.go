// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package middlewares_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/transport/http/server/middlewares"
)

func names(ms []middlewares.Middleware) []string {
	out := make([]string, len(ms))
	for i, m := range ms {
		out[i] = m.Name()
	}
	return out
}

// FuncWithDeps declares its dependencies to the ordering: a middleware listed
// first still runs after the one it depends on.
func TestOrderMiddlewares_FuncWithDeps(t *testing.T) {
	t.Parallel()

	pass := func(next http.Handler) http.Handler { return next }
	auth := middlewares.FuncWithDeps("auth", []string{"logger"}, pass)
	logger := middlewares.Func("logger", pass)

	ordered, err := middlewares.OrderMiddlewares([]middlewares.Middleware{auth, logger})
	require.NoError(t, err)
	require.Equal(t, []string{"logger", "auth"}, names(ordered))
}
