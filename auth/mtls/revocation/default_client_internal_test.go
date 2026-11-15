// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package revocation

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// The check context already bounds a query, so only the client's own field
// shows that the default client carries the check timeout.
func TestDefaultClientHasTimeout(t *testing.T) {
	t.Parallel()

	require.Equal(t, DefaultTimeout, New(nil).httpClient.Timeout)
	require.Equal(t, 123*time.Millisecond, New(nil, WithTimeout(123*time.Millisecond)).httpClient.Timeout)

	custom := &http.Client{}
	require.Same(t, custom, New(nil, WithHTTPClient(custom), WithTimeout(time.Second)).httpClient)
	require.Zero(t, custom.Timeout, "a supplied client is used as is")
}
