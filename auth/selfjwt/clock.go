// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package selfjwt

import "time"

// Clock returns the current time. It is an injectable seam so tests can pin the
// wall clock; production uses the system UTC clock.
type Clock interface {
	Now() time.Time
}

// systemClock is the production Clock, returning the current UTC time.
type systemClock struct{}

// Now returns the current UTC time.
func (systemClock) Now() time.Time { return time.Now().UTC() }

// defaultClock is the production wall clock used for token timing.
var defaultClock Clock = systemClock{}
