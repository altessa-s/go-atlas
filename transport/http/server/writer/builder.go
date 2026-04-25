// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package writer

import "net/http"

// Builder constructs response structures from arbitrary data.
// Implementations determine how data is wrapped and what HTTP status code to use.
type Builder interface {
	// Build transforms data into a response structure with an HTTP status code.
	// For successful responses, data is typically wrapped in a success structure.
	// For errors, data is converted to an error response structure.
	Build(req *http.Request, data any) (response any, statusCode int)
}
