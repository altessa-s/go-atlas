// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package singleflight

//go:generate go run github.com/altessa-s/go-atlas/cmd/optgen generate

import "time"

// DefaultMaxConcurrent limits distinct computations admitted at once.
const DefaultMaxConcurrent = 1024

// DefaultTimeout bounds the context passed to a shared computation.
const DefaultTimeout = 15 * time.Second

type options struct {
	maxConcurrent int           `optgen:"default=DefaultMaxConcurrent"`
	timeout       time.Duration `optgen:"default=DefaultTimeout"`
}
