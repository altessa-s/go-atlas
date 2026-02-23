// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package appstats

import "time"

var start time.Time

func init() {
	start = time.Now().UTC()
}

// Uptime returns the duration since the application started.
// The value is calculated from the time when the package was initialized.
// The returned duration can be used to determine how long the application
// has been running.
func Uptime() time.Duration {
	return time.Since(start)
}
