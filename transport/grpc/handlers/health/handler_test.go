// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package health

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/observability/health"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
)

var _ HealthWatcher = (*health.Coordinator)(nil)

type fakeWatcher struct {
	status   health.ServingStatus
	statuses map[string]health.ServingStatus
	err      error
}

func (f fakeWatcher) CheckStatus(context.Context, string) health.ServingStatus { return f.status }
func (f fakeWatcher) ListStatuses(context.Context) (map[string]health.ServingStatus, error) {
	return f.statuses, f.err
}
func (f fakeWatcher) Subscribe(context.Context, string) (health.Subscriber, error) {
	return nil, errors.New("unused")
}

func TestNew(t *testing.T) {
	t.Parallel()
	coord := health.New()
	h := New(coord)
	require.NotNil(t, h, "New returned nil")
}

func TestHealthWatcherSeam(t *testing.T) {
	t.Parallel()
	h := New(fakeWatcher{status: health.StatusServing, statuses: map[string]health.ServingStatus{"api": health.StatusServing}})
	resp, err := h.Check(t.Context(), &grpc_health_v1.HealthCheckRequest{})
	require.NoError(t, err)
	require.Equal(t, grpc_health_v1.HealthCheckResponse_SERVING, resp.Status)
	list, err := h.List(t.Context(), &grpc_health_v1.HealthListRequest{})
	require.NoError(t, err)
	require.Equal(t, grpc_health_v1.HealthCheckResponse_SERVING, list.Statuses["api"].Status)

	listErr := errors.New("list failed")
	_, err = New(fakeWatcher{err: listErr}).List(t.Context(), &grpc_health_v1.HealthListRequest{})
	require.Equal(t, codes.Internal, status.Code(err))
}

func TestList_SanitizesErrors(t *testing.T) {
	t.Parallel()

	t.Run("backend error hidden", func(t *testing.T) {
		t.Parallel()
		secretErr := errors.New("dial 10.0.0.5:6379: secret")
		_, err := New(fakeWatcher{err: secretErr}).List(t.Context(), &grpc_health_v1.HealthListRequest{})
		require.Error(t, err)
		st, ok := status.FromError(err)
		require.True(t, ok)
		require.Equal(t, codes.Internal, st.Code())
		require.NotContains(t, st.Message(), "10.0.0.5")
		require.NotContains(t, st.Message(), "secret")
		require.False(t, errors.Is(err, secretErr))
	})

	t.Run("coordinator shutdown", func(t *testing.T) {
		t.Parallel()
		shutdownErr := fmt.Errorf("list: %w", health.ErrCoordinatorShutdown)
		_, err := New(fakeWatcher{err: shutdownErr}).List(t.Context(), &grpc_health_v1.HealthListRequest{})
		require.Error(t, err)
		require.Equal(t, codes.Unavailable, status.Code(err))
		require.False(t, errors.Is(err, shutdownErr))
	})
}
