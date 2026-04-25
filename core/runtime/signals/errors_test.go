// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package signals

import (
	"errors"
	"fmt"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestTimeoutError(t *testing.T) {
	err := &TimeoutError{Signal: syscall.SIGTERM, Timeout: 5 * time.Second}
	require.NotEmpty(t, err.Error())
	require.True(t, IsTimeout(err))
}

func TestPanicError(t *testing.T) {
	err := &PanicError{Signal: syscall.SIGINT, Panic: "something broke"}
	require.NotEmpty(t, err.Error())
}

func TestIsTimeout(t *testing.T) {
	require.False(t, IsTimeout(fmt.Errorf("regular error")))
	require.False(t, IsTimeout(nil))

	wrapped := fmt.Errorf("wrapped: %w", &TimeoutError{Signal: syscall.SIGTERM, Timeout: time.Second})
	require.True(t, IsTimeout(wrapped))

	require.False(t, IsTimeout(errors.New("plain")))
}
