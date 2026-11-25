// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package server_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/transport/http/server"
	"github.com/altessa-s/go-atlas/transport/http/server/router/std"
)

type delayedHandler struct {
	entered chan (<-chan struct{})
	resume  chan struct{}
}

func (h delayedHandler) Register(_ server.RouteRegistrar, stop <-chan struct{}) {
	h.entered <- stop
	<-h.resume
}

func TestShutdownSignalsHandlersDuringRegistration(t *testing.T) {
	t.Parallel()
	srv, err := server.New(server.WithRouter(std.New()))
	require.NoError(t, err)
	handler := delayedHandler{entered: make(chan (<-chan struct{}), 1), resume: make(chan struct{})}
	t.Cleanup(func() { close(handler.resume) })
	srv.RegisterHandlers(handler)
	started := make(chan error, 1)
	go func() { started <- srv.Start() }()
	select {
	case stop := <-handler.entered:
		require.NoError(t, srv.Shutdown(t.Context()))
		select {
		case <-stop:
		default:
			require.FailNow(t, "handler did not receive its stop signal")
		}
	case err := <-started:
		require.FailNow(t, "server returned before registering the handler", "%v", err)
	}
	handler.resume <- struct{}{}
	require.ErrorIs(t, <-started, server.ErrServerClosed)
}
