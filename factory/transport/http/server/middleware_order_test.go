// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/transport/http/server/middlewares"
)

type outerMiddleware struct{ middlewares.Middleware }

func (outerMiddleware) Outermost() bool { return true }

func TestOutermostMiddlewareOrder(t *testing.T) {
	t.Parallel()
	id := middlewares.Noop("requestid")
	logger := middlewares.Noop("logger")
	guard := outerMiddleware{middlewares.Noop("guard")}
	for _, tc := range []struct {
		name  string
		input []middlewares.Middleware
		want  []string
	}{
		{"moves to head", []middlewares.Middleware{id, logger, guard}, []string{"guard", "requestid", "logger"}},
		{"already first", []middlewares.Middleware{guard, id}, []string{"guard", "requestid"}},
		{"name alone", []middlewares.Middleware{id, middlewares.Noop("recovery")}, []string{"requestid", "recovery"}},
		{"conditional", []middlewares.Middleware{id, middlewares.ConditionalMiddleware(middlewares.MatchFunc(func() bool { return true }), guard)},
			[]string{"guard", "requestid"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			collection := middlewareCollection{additionalMW: tc.input}
			ordered, err := collection.ordered()
			require.NoError(t, err)
			names := make([]string, 0, len(ordered))
			for _, mw := range ordered {
				names = append(names, mw.Name())
			}
			require.Equal(t, tc.want, names)
		})
	}
}
