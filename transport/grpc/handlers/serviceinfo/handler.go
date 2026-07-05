// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package serviceinfo

import (
	"context"
	"maps"
	"runtime"
	"time"

	"github.com/altessa-s/go-atlas/core/runtime/appinfo"
	"github.com/altessa-s/go-atlas/core/runtime/panics"
	"github.com/altessa-s/go-atlas/core/text/strings"
	"github.com/altessa-s/go-atlas/core/types/ptr"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	serviceinfov1 "github.com/altessa-s/proto-gen-go/io/altessa/serviceinfo/v1"
	stdGrpc "google.golang.org/grpc"
	stdstrings "strings"
)

// Handler implements [serviceinfov1.ServiceInfoServiceServer]. The
// static portion of the response is built once in [New]; per-request
// dynamic fields (leader, leader_id, start_time, uptime) are layered
// on top of a defensive [proto.Clone] of the cached message.
type Handler struct {
	serviceinfov1.UnimplementedServiceInfoServiceServer

	leaderProvider LeaderProvider
	serviceID      string
	startTime      time.Time
	static         *serviceinfov1.ServiceInfo
}

// New constructs a [Handler]. With no options, all fields are populated
// from [appinfo] (which the host service is expected to populate via
// -ldflags at build time).
func New(opts ...Option) *Handler {
	o := newOptions(opts...)
	return &Handler{
		leaderProvider: o.leaderProvider,
		serviceID:      o.serviceID,
		startTime:      time.Now().UTC(),
		static:         buildStaticInfo(o),
	}
}

// Register attaches the handler to gs. The stop channel is unused —
// Get is unary and has no streams to terminate on shutdown.
func (h *Handler) Register(gs *stdGrpc.Server, _ <-chan struct{}) {
	serviceinfov1.RegisterServiceInfoServiceServer(gs, h)
}

// Get returns the cached static info plus dynamic per-request fields.
// The returned pointer is safe to mutate by the caller; each call
// produces a fresh deep copy via [proto.Clone].
func (h *Handler) Get(ctx context.Context, _ *emptypb.Empty) (*serviceinfov1.ServiceInfo, error) {
	out, ok := proto.Clone(h.static).(*serviceinfov1.ServiceInfo)
	panics.Must(ok, "proto.Clone returned an unexpected concrete type")

	if h.leaderProvider != nil {
		out.Leader = h.leaderProvider.IsLeader()
		leaderID := h.serviceID
		if !out.Leader {
			if id, err := h.leaderProvider.LeaderId(ctx); err == nil && id != "" {
				leaderID = id
			}
		}
		if leaderID != "" {
			out.LeaderId = ptr.Wrap(leaderID)
		}
	}

	out.StartTime = timestamppb.New(h.startTime)
	out.Uptime = durationpb.New(time.Since(h.startTime))

	return out, nil
}

// buildStaticInfo assembles the immutable portion of the ServiceInfo
// message from appinfo and the configured options.
func buildStaticInfo(o *options) *serviceinfov1.ServiceInfo {
	semver := appinfo.SemVersion()

	info := &serviceinfov1.ServiceInfo{
		ServiceName:        o.serviceName,
		ServiceDescription: ptr.WrapNonZero(o.serviceDescription),
		ServiceId:          ptr.WrapNonZero(o.serviceID),
		FullVersion:        appinfo.Version,
		BuildTime:          buildTimestamp(),
		Branch:             ptr.WrapNonZero(appinfo.Branch),
		Commit:             ptr.WrapNonZero(appinfo.Commit),
		BuildTags:          ptr.WrapNonZero(appinfo.BuildTags()),
		Metadata:           buildMetadata(o.extraMetadata),
		SemanticVersion: &serviceinfov1.ServiceInfo_SemanticVersion{
			Major:      strings.ToUint32(semver.Major),
			Minor:      strings.ToUint32(semver.Minor),
			Patch:      strings.ToUint32(semver.Patch),
			PreRelease: ptr.WrapNonZero(stdstrings.Join(semver.Prerelease, ".")),
		},
	}

	return info
}

// buildTimestamp parses appinfo.BuildTime (RFC 3339) into a protobuf
// timestamp, returning nil when it is unset or malformed.
func buildTimestamp() *timestamppb.Timestamp {
	t, err := time.Parse(time.RFC3339, appinfo.BuildTime)
	if err != nil {
		return nil
	}
	return timestamppb.New(t)
}

// buildMetadata composes the metadata map from runtime/build values and
// the caller-supplied extras. Caller-supplied keys win on conflict so a
// service can override defaults (e.g. swap "go_version" for a custom
// build tag).
func buildMetadata(extra map[string]string) map[string]string {
	m := map[string]string{
		"go_version":       runtime.Version(),
		"go_os":            runtime.GOOS,
		"go_arch":          runtime.GOARCH,
		"build_go_version": appinfo.BuildGoVersion(),
		"build_go_os":      appinfo.BuildGoOS(),
		"build_go_arch":    appinfo.BuildGoArch(),
		"cgo_enabled":      boolFlag(appinfo.IsCgoEnabled()),
		"race_enabled":     boolFlag(appinfo.IsRaceEnabled()),
	}
	maps.Copy(m, extra)
	return m
}

func boolFlag(b bool) string {
	if b {
		return "1"
	}
	return "0"
}
