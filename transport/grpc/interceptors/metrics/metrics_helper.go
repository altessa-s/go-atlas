// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package metrics

import (
	"sync"
	"time"

	"github.com/altessa-s/go-atlas-core/text/strings/intern"
	"github.com/altessa-s/go-atlas/observability/metrics"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/internal/callstatus"

	"google.golang.org/grpc/codes"
)

// methodHandles caches the bound label handles for a single gRPC method so
// the per-request path reuses them instead of building a label map and a
// labeled wrapper on every observation. Cardinality is bounded by
// [WithMaxMethods] (times the gRPC status-code set for byStatus).
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
// Once [WithMaxMethods] distinct methods are tracked, any further method
// shares the handles of [OverflowMethod].
func (i *interceptor) methodFor(method string) *methodHandles {
	i.methodMu.RLock()
	mh := i.byMethod[method]
	if mh == nil {
		mh = i.overflow
	}
	i.methodMu.RUnlock()
	if mh != nil {
		return mh
	}

	i.methodMu.Lock()
	defer i.methodMu.Unlock()
	if mh = i.byMethod[method]; mh != nil {
		return mh
	}
	if len(i.byMethod) >= i.opts.maxMethods {
		if i.overflow == nil {
			i.overflow = i.bindMethod(OverflowMethod)
		}
		return i.overflow
	}
	mh = i.bindMethod(method)
	i.byMethod[method] = mh
	return mh
}

func (i *interceptor) bindMethod(method string) *methodHandles {
	mh := &methodHandles{
		method:   method,
		inFlight: i.requestsInFlightByMethod.WithLabels(metrics.Labels{methodLabel: method}),
	}
	if i.streamMessageSize != nil {
		mh.msgSizeSent = i.streamMessageSize.WithLabels(metrics.Labels{methodLabel: method, directionLabel: directionSent})
		mh.msgSizeRecv = i.streamMessageSize.WithLabels(metrics.Labels{methodLabel: method, directionLabel: directionReceived})
	}
	return mh
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
func (i *interceptor) recordMetrics(mh *methodHandles, startTime time.Time, req, resp any, code codes.Code) {
	duration := time.Since(startTime)

	h := i.statusFor(mh, intern.String(code.String()))

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
	return callstatus.Status(err).Code()
}
