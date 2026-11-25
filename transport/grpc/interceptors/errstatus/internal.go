// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package errstatus

import "reflect"

// errorTypeString returns a string representation of the error type for debugging.
func errorTypeString(err error) string {
	if err == nil {
		return "<nil>"
	}
	return reflect.TypeOf(err).String()
}
