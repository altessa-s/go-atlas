// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package outbox

import (
	"errors"
	"testing"
)

func BenchmarkEvent_NextAttempt(b *testing.B) {
	e := &Event{}
	for b.Loop() {
		e.nextAttempt()
	}
}

func BenchmarkEvent_SetErrorStatus(b *testing.B) {
	err := errors.New("fail")
	e := &Event{}
	for b.Loop() {
		e.setErrorStatus(err)
	}
}

func BenchmarkEvent_SetSentStatus(b *testing.B) {
	e := &Event{}
	for b.Loop() {
		e.setSentStatus()
	}
}

func BenchmarkEvent_IsReadyForRetry(b *testing.B) {
	e := &Event{Attempts: 5}
	for b.Loop() {
		e.isReadyForRetry(10)
	}
}
