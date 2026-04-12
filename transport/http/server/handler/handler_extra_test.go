// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package handler_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/transport/http/server/handler"
	"github.com/altessa-s/go-atlas/transport/http/server/router"
)

type mockRoute struct{}

func (m mockRoute) Handler(http.Handler) router.Route                                 { return m }
func (m mockRoute) HandlerFunc(func(http.ResponseWriter, *http.Request)) router.Route { return m }
func (m mockRoute) Methods(...string) router.Route                                    { return m }
func (m mockRoute) Path(string) router.Route                                          { return m }
func (m mockRoute) PathPrefix(string) router.Route                                    { return m }
func (m mockRoute) Subrouter() router.Router                                          { return &mockRouter{} }

type mockRouter struct{}

func (m *mockRouter) Handle(string, http.Handler) router.Route { return mockRoute{} }
func (m *mockRouter) HandleFunc(string, func(http.ResponseWriter, *http.Request)) router.Route {
	return mockRoute{}
}
func (m *mockRouter) Methods(...string) router.Route               { return mockRoute{} }
func (m *mockRouter) PathPrefix(string) router.Route               { return mockRoute{} }
func (m *mockRouter) Use(...router.Middleware)                     {}
func (m *mockRouter) ServeHTTP(http.ResponseWriter, *http.Request) {}
func (m *mockRouter) Subrouter() router.Router                     { return m }

func TestPrometheusMetrics(t *testing.T) {
	r := &mockRouter{}
	result := handler.PrometheusMetrics(r)
	require.NotNil(t, result)
}

func TestPprof(t *testing.T) {
	r := &mockRouter{}
	result := handler.Pprof(r)
	require.NotNil(t, result)
}
