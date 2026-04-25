// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package health

import (
	"context"

	"github.com/altessa-s/go-atlas/core/runtime/panics"
	"github.com/altessa-s/go-atlas/observability/health"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"

	stdGrpc "google.golang.org/grpc"
)

// Handler implements grpc_health_v1.HealthServer by delegating to a shared
// [health.Coordinator]. The coordinator may be used concurrently by other
// subsystems (HTTP health endpoints, readiness probes, etc.).
type Handler struct {
	grpc_health_v1.UnimplementedHealthServer

	coordinator *health.Coordinator
	stop        <-chan struct{}
}

// New creates a Handler backed by coord. Panics if coord is nil.
func New(coord *health.Coordinator) *Handler {
	panics.MustNonNil(coord, "coordinator must not be nil")
	return &Handler{coordinator: coord}
}

// Register attaches the handler to gs and stores stop for Watch stream
// termination. Must be called before the server starts accepting connections.
func (h *Handler) Register(gs *stdGrpc.Server, stop <-chan struct{}) {
	h.stop = stop
	grpc_health_v1.RegisterHealthServer(gs, h)
}

// Check returns the serving status of the named service, or the overall
// server status when the service name is empty. Returns codes.NotFound
// for unrecognized service names.
func (h *Handler) Check(ctx context.Context, req *grpc_health_v1.HealthCheckRequest) (*grpc_health_v1.HealthCheckResponse, error) {
	service := req.GetService()
	servingStatus := h.coordinator.CheckStatus(ctx, service)

	if service != "" && servingStatus == health.StatusServiceUnknown {
		return nil, status.Errorf(codes.NotFound, "unknown service: %s", service)
	}

	return &grpc_health_v1.HealthCheckResponse{Status: ToProto(servingStatus)}, nil
}

// List gathers health statuses for all services.
func (h *Handler) List(ctx context.Context, _ *grpc_health_v1.HealthListRequest) (*grpc_health_v1.HealthListResponse, error) {
	statuses, err := h.coordinator.ListStatuses(ctx)
	if err != nil {
		return nil, err
	}

	resp := &grpc_health_v1.HealthListResponse{Statuses: make(map[string]*grpc_health_v1.HealthCheckResponse, len(statuses))}
	for service, servingStatus := range statuses {
		resp.Statuses[service] = &grpc_health_v1.HealthCheckResponse{Status: ToProto(servingStatus)}
	}

	return resp, nil
}

// Watch opens a server-streaming subscription for the named service.
// It sends the current status immediately, then pushes changes until the
// client disconnects, the stop channel closes (codes.Canceled), or the
// coordinator signals codes.ResourceExhausted when watcher limits are hit.
// Duplicate consecutive statuses are suppressed.
func (h *Handler) Watch(req *grpc_health_v1.HealthCheckRequest, stream stdGrpc.ServerStreamingServer[grpc_health_v1.HealthCheckResponse]) error {
	service := req.GetService()
	ctx := stream.Context()

	subscription, err := h.coordinator.Subscribe(ctx, service)
	if err != nil {
		return status.Errorf(codes.ResourceExhausted, "too many watchers for service: %s", service)
	}
	defer subscription.Close()

	lastStatus := subscription.InitialStatus()
	if err = stream.Send(&grpc_health_v1.HealthCheckResponse{Status: ToProto(lastStatus)}); err != nil {
		return err
	}

	updates := subscription.Updates()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-h.stop:
			return status.New(codes.Canceled, "Server shutting down").Err()
		case newStatus, ok := <-updates:
			if !ok {
				return nil
			}

			if newStatus == lastStatus {
				continue
			}

			lastStatus = newStatus
			if err = stream.Send(&grpc_health_v1.HealthCheckResponse{Status: ToProto(newStatus)}); err != nil {
				return err
			}
		}
	}
}
