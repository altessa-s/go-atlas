// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package audit_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/transport/depgraph"
	"github.com/altessa-s/go-atlas/transport/http/server/middlewares"
	"github.com/altessa-s/go-atlas/transport/http/server/middlewares/realip"
	"github.com/altessa-s/go-atlas/transport/http/server/middlewares/requestid"

	audithttp "github.com/altessa-s/go-atlas/transport/http/server/middlewares/audit"
	tracingmw "github.com/altessa-s/go-atlas/transport/http/server/middlewares/tracing"
)

// The audit middleware reads the client IP, request ID and trace from the
// context, so the chain ordering must run those middlewares first; realip
// is required, the others optional.
func TestAudit_ChainOrdering(t *testing.T) {
	t.Parallel()

	pass := func(next http.Handler) http.Handler { return next }
	stand := func(name string) middlewares.Middleware { return middlewares.Func(name, pass) }
	mw := audithttp.New(nil)

	ordered, err := middlewares.OrderMiddlewares([]middlewares.Middleware{
		mw, stand(tracingmw.Name()), stand(requestid.Name()), stand(realip.Name()),
	})
	require.NoError(t, err)
	require.Len(t, ordered, 4)
	require.Equal(t, mw.Name(), ordered[len(ordered)-1].Name(), "audit runs after the middlewares it reads from")

	_, err = middlewares.OrderMiddlewares([]middlewares.Middleware{mw, stand(realip.Name())})
	require.NoError(t, err, "request ID and tracing are optional")

	_, err = middlewares.OrderMiddlewares([]middlewares.Middleware{mw, stand(requestid.Name())})
	require.ErrorIs(t, err, depgraph.ErrMissingDependency, "realip is required")
}
