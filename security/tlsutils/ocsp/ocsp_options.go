// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package ocsp

//go:generate go run github.com/altessa-s/go-atlas/cmd/optgen generate

import (
	"log/slog"
	"net/http"

	corescheduler "github.com/altessa-s/go-atlas/core/scheduler"
)

type options struct {
	httpClient        *http.Client
	retryPolicy       RetryPolicy `optgen:"notnil"`
	logger            *slog.Logger
	enableCompression bool `opt:"Compression"`

	// Scheduler configuration
	scheduler       corescheduler.TaskRegistrar `optgen:"notnil"`
	refreshSchedule string
}
