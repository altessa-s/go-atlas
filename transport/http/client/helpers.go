// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package client

import (
	"io"
)

// CloseBody drains and closes an HTTP response body so the underlying TCP
// connection can be reused by the transport connection pool. It is a no-op
// when body is nil and all errors are intentionally discarded — this helper
// is meant for defer statements in call sites that do not care about the
// cleanup result.
//
// Typical usage:
//
//	resp, err := httpClient.Do(req)
//	if err != nil {
//	    return err
//	}
//	defer client.CloseBody(resp.Body)
func CloseBody(body io.ReadCloser) {
	if body == nil {
		return
	}
	_, _ = io.Copy(io.Discard, body)
	_ = body.Close()
}
