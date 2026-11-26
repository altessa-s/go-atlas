// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"sync"
	"testing"
	"testing/synctest"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-slogx/setup"

	observabilityconfig "github.com/altessa-s/go-atlas/config/observability"
)

func TestBufferBypassLevel(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		level   observabilityconfig.LoggerLevel
		returns bool
	}{
		{level: observabilityconfig.LoggerLevelNone, returns: true},
		{level: observabilityconfig.LoggerLevelError, returns: false},
	} {
		t.Run(string(tt.level), func(t *testing.T) {
			t.Parallel()
			synctest.Test(t, func(t *testing.T) {
				cfg := &observabilityconfig.Logger{
					Level: observabilityconfig.LoggerLevelInfo,
					Buffer: observabilityconfig.LoggerBuffer{
						Enabled: true, BypassLevel: tt.level,
					},
				}
				release := make(chan struct{})
				w := blockedLogWriter{release: release, entered: make(chan struct{})}
				logger, err := setup.New(w, New(cfg).options(nil)...)
				require.NoError(t, err)
				unblock := sync.OnceFunc(func() { close(release) })
				defer func() {
					unblock()
					_, err := logger.Shutdown(t.Context())
					require.NoError(t, err)
				}()

				done := make(chan struct{})
				go func() {
					logger.Logger().Error("buffered error")
					close(done)
				}()
				<-w.entered
				synctest.Wait()
				var returned bool
				select {
				case <-done:
					returned = true
				default:
				}
				unblock()
				<-done
				require.Equal(t, tt.returns, returned, "whether the logging call returns while the writer is blocked")
			})
		})
	}
}

type blockedLogWriter struct {
	entered chan struct{}
	release <-chan struct{}
}

func (w blockedLogWriter) Write(p []byte) (int, error) {
	close(w.entered)
	<-w.release
	return len(p), nil
}
