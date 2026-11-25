// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package architecture_test

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/scope"
	"github.com/altessa-s/go-atlas/transport/grpc/client"
	"github.com/altessa-s/go-atlas/transport/grpc/handlers/scheduler"

	grpcauth "github.com/altessa-s/go-atlas/transport/grpc/interceptors/auth"
	grpcgeo "github.com/altessa-s/go-atlas/transport/grpc/interceptors/geoacl"
	grpcip "github.com/altessa-s/go-atlas/transport/grpc/interceptors/ipacl"
	httpauth "github.com/altessa-s/go-atlas/transport/http/server/middlewares/auth"
	httpgeo "github.com/altessa-s/go-atlas/transport/http/server/middlewares/geoacl"
	httpip "github.com/altessa-s/go-atlas/transport/http/server/middlewares/ipacl"
)

// TestTransportConsumerPorts checks the actual input at each public construction
// site so a concrete dependency cannot return behind an import or type alias.
func TestTransportConsumerPorts(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name        string
		constructor any
		param       int
	}{
		{"registry", scope.NewEnforcer[string], 0},
		{"grpc scope", grpcauth.ScopeClientAuth[string], 0},
		{"http scope", httpauth.ScopeMiddleware[string], 0},
		{"scheduler", scheduler.New, 0},
		{"grpc IP", grpcip.ServerInterceptor, 0},
		{"http IP", httpip.New, 0},
		{"grpc geo", grpcgeo.ServerInterceptor, 1},
		{"http geo", httpgeo.New, 1},
		{"pool", client.WithPool, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			param := reflect.TypeOf(tc.constructor).In(tc.param)
			require.Equal(t, reflect.Interface, param.Kind(), "constructor takes %s", param)
		})
	}
}

func TestRequestIDIndependentOfTracing(t *testing.T) {
	t.Parallel()
	walkProductionImports(t, func(file, pkg, dependency string) {
		if pkg == "transport/requestid" && hasPathPrefix(dependency, []string{module + "observability/tracing"}) {
			t.Errorf("%s imports %s", file, dependency)
		}
	})
}
