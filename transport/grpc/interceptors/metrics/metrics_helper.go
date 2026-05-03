// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package metrics

import (
	"time"

	"github.com/altessa-s/go-atlas/core/text/strings"
	"github.com/altessa-s/go-atlas/observability/metrics"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
	prominternal "github.com/altessa-s/go-atlas/transport/internal/prometheus"
)

// metricsRecorder provides common metrics recording functionality.
type metricsRecorder struct {
	requestsTotal   metrics.Counter
	requestDuration metrics.Histogram
	requestSize     metrics.Histogram
	responseSize    metrics.Histogram
	opts            *options
	logPrefix       string
}

// record records metrics for a completed request.
func (r *metricsRecorder) record(fullMethod string, startTime time.Time, req, resp any, err error) {
	duration := time.Since(startTime)
	statusCode := getStatusCode(err)

	internedMethod := strings.InternString(fullMethod)
	internedStatus := strings.InternString(statusCode.String())

	labels := metrics.Labels{
		methodLabel: internedMethod,
		statusLabel: internedStatus,
	}

	r.requestsTotal.WithLabels(labels).Inc()
	r.requestDuration.WithLabels(labels).Observe(duration.Seconds())

	if r.opts.enableSizeMetrics && r.requestSize != nil && r.responseSize != nil {
		if req != nil {
			if size, ok := prominternal.GetMessageSize(req); ok {
				r.requestSize.WithLabels(labels).Observe(float64(size))
			}
		}
		if resp != nil {
			if size, ok := prominternal.GetMessageSize(resp); ok {
				r.responseSize.WithLabels(labels).Observe(float64(size))
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
