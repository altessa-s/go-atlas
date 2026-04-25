// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package prometheus

import (
	"time"

	"github.com/altessa-s/go-atlas/core/text/strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
	prominternal "github.com/altessa-s/go-atlas/transport/internal/prometheus"
	prom "github.com/prometheus/client_golang/prometheus"
)

// metricsRecorder provides common metrics recording functionality
// for both server and client interceptors.
type metricsRecorder struct {
	requestsTotal   *prom.CounterVec
	requestDuration *prom.HistogramVec
	requestSize     *prom.HistogramVec
	responseSize    *prom.HistogramVec
	opts            *options
	logPrefix       string
}

// record records metrics for a completed request.
func (r *metricsRecorder) record(fullMethod string, startTime time.Time, req, resp any, err error) {
	duration := time.Since(startTime)
	statusCode := getStatusCode(err)
	statusString := statusCode.String()

	// Intern strings to reduce memory usage for Prometheus labels
	internedMethod := strings.InternString(fullMethod)
	internedStatus := strings.InternString(statusString)

	// Record core metrics
	r.requestsTotal.WithLabelValues(internedMethod, internedStatus).Inc()
	r.requestDuration.WithLabelValues(internedMethod, internedStatus).Observe(duration.Seconds())

	// Record optional size metrics
	if r.opts.enableSizeMetrics && r.requestSize != nil && r.responseSize != nil {
		if req != nil {
			if size, ok := prominternal.GetMessageSize(req); ok {
				r.requestSize.WithLabelValues(internedMethod, internedStatus).Observe(float64(size))
			}
		}
		if resp != nil {
			if size, ok := prominternal.GetMessageSize(resp); ok {
				r.responseSize.WithLabelValues(internedMethod, internedStatus).Observe(float64(size))
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
