// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package httpsign

import "errors"

// ErrBodyTooLarge indicates the request body exceeded the configured limit
// ([WithMaxBytes]) before it could be verified. The default error handler maps
// it to 413 Request Entity Too Large; all other verification failures map to
// 401. Match with errors.Is.
var ErrBodyTooLarge = errors.New("httpsign: request body too large")
