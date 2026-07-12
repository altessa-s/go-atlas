// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package metrics

import (
	"sync"
	"time"

	"github.com/altessa-s/go-atlas/core/text/strings"
	"github.com/altessa-s/go-atlas/observability/metrics"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// methodHandles caches the bound label handles for a single gRPC method so
// the per-request path reuses them instead of building a label map and a
// labeled wrapper on every observation. Cardinality is bounded by the
// service's method set (times the gRPC status-code set for byStatus).
type methodHandles struct {
	method      string
	inFlight    metrics.Gauge
	msgSizeSent metrics.Histogram // nil unless stream+size metrics enabled
	msgSizeRecv metrics.Histogram
	byStatus    sync.Map // status string → *statusHandles
}

// statusHandles caches the bound handles for one (method, status) pair.
type statusHandles struct {
	total      metrics.Counter
	duration   metrics.Histogram
	reqSize    metrics.Histogram // nil unless size metrics enabled
	respSize   metrics.Histogram
	streamSent metrics.Counter // nil unless stream metrics enabled
	streamRecv metrics.Counter
}

// methodFor returns the cached handles for method, binding them on first use.
func (i *interceptor) methodFor(method string) *methodHandles {
	if v, ok := i.byMethod.Load(method); ok {
		return v.(*methodHandles) //nolint:errcheck // only *methodHandles is stored
	}
	mh := &methodHandles{
		method:   method,
		inFlight: i.requestsInFlightByMethod.WithLabels(metrics.Labels{methodLabel: method}),
	}
	if i.streamMessageSize != nil {
		mh.msgSizeSent = i.streamMessageSize.WithLabels(metrics.Labels{methodLabel: method, directionLabel: directionSent})
		mh.msgSizeRecv = i.streamMessageSize.WithLabels(metrics.Labels{methodLabel: method, directionLabel: directionReceived})
	}
	v, _ := i.byMethod.LoadOrStore(method, mh)
	return v.(*methodHandles) //nolint:errcheck // only *methodHandles is stored
}

// statusFor returns the cached handles for (method, status), binding on first use.
func (i *interceptor) statusFor(mh *methodHandles, statusCode string) *statusHandles {
	if v, ok := mh.byStatus.Load(statusCode); ok {
		return v.(*statusHandles) //nolint:errcheck // only *statusHandles is stored
	}
	labels := metrics.Labels{methodLabel: mh.method, statusLabel: statusCode}
	sh := &statusHandles{
		total:    i.requestsTotal.WithLabels(labels),
		duration: i.requestDuration.WithLabels(labels),
	}
	if i.requestSize != nil {
		sh.reqSize = i.requestSize.WithLabels(labels)
	}
	if i.responseSize != nil {
		sh.respSize = i.responseSize.WithLabels(labels)
	}
	if i.streamMessagesSent != nil {
		sh.streamSent = i.streamMessagesSent.WithLabels(labels)
	}
	if i.streamMessagesReceived != nil {
		sh.streamRecv = i.streamMessagesReceived.WithLabels(labels)
	}
	v, _ := mh.byStatus.LoadOrStore(statusCode, sh)
	return v.(*statusHandles) //nolint:errcheck // only *statusHandles is stored
}

// recordMetrics records metrics for a completed request.
func (i *interceptor) recordMetrics(mh *methodHandles, startTime time.Time, req, resp any, err error) {
	duration := time.Since(startTime)
	statusCode := getStatusCode(err)

	h := i.statusFor(mh, strings.InternString(statusCode.String()))

	h.total.Inc()
	h.duration.Observe(duration.Seconds())

	if i.opts.enableSizeMetrics && h.reqSize != nil && h.respSize != nil {
		if req != nil {
			if size, ok := getMessageSize(req); ok {
				h.reqSize.Observe(float64(size))
			}
		}
		if resp != nil {
			if size, ok := getMessageSize(resp); ok {
				h.respSize.Observe(float64(size))
			}
		}
	}
}

// getStatusCode extracts the gRPC status code from an error.
func getStatusCode(err error) codes.Code {
	if err == nil {
		return codes.OK
	}

	if coreerrs.IsContextCanceledOrDeadlineExceeded(err) {
		return codes.DeadlineExceeded
	}

	if st, ok := status.FromError(err); ok {
		return st.Code()
	}

	return codes.Internal
}
