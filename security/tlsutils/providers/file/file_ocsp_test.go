// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package tlsfile_test

import (
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"
	"github.com/altessa-s/go-atlas/security/tlsutils/providers/internal/staplertest"

	tlsfile "github.com/altessa-s/go-atlas/security/tlsutils/providers/file"
)

// TestFile_OCSPStaplingAppliedOnce guards against the constructor wrapping an
// already-stapled config a second time: one handshake must hit the stapler once.
func TestFile_OCSPStaplingAppliedOnce(t *testing.T) {
	t.Parallel()
	certPEM, keyPEM, _ := testhelpers.SelfSignedCert(t)
	certPath, keyPath := testhelpers.WriteTempCertFiles(t, certPEM, keyPEM)
	stapler := &staplertest.CountingStapler{}

	f, err := tlsfile.NewWithCertAndKey(certPath, keyPath, "", tlsfile.WithOcspStapler(stapler))
	require.NoError(t, err)
	t.Cleanup(func() { _ = f.Close(t.Context()) })

	cfg, err := f.TLSConfig()
	require.NoError(t, err)
	require.Equal(t, staplertest.Staple, staplertest.Handshake(t, cfg))
	require.Equal(t, int32(1), stapler.Calls())
}

// TestFile_ReloadPublishesFullyStapledConfig reloads the certificate while
// readers clone the published config. Under -race this catches a config that
// is mutated after publication; every clone must carry the stapling callback.
func TestFile_ReloadPublishesFullyStapledConfig(t *testing.T) {
	t.Parallel()
	certPEM, keyPEM, _ := testhelpers.SelfSignedCert(t)
	certPath, keyPath := testhelpers.WriteTempCertFiles(t, certPEM, keyPEM)
	notify := make(chan struct{}, 1)

	f, err := tlsfile.NewWithCertAndKey(certPath, keyPath, "",
		tlsfile.WithOcspStapler(&staplertest.CountingStapler{}),
		tlsfile.WithEnableWatcher(),
		tlsfile.WithReloadNotifyChan(notify),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = f.Close(t.Context()) })
	<-notify // initial load

	stop := make(chan struct{})
	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			for {
				select {
				case <-stop:
					return
				default:
				}
				cfg, cerr := f.TLSConfig()
				if cerr != nil || cfg.GetCertificate == nil {
					t.Error("published TLS config lacks the OCSP stapling callback")
					return
				}
			}
		})
	}

	const reloads = 3
	for range reloads {
		newCert, newKey, _ := testhelpers.SelfSignedCert(t)
		require.NoError(t, os.WriteFile(keyPath, newKey, 0o600))
		require.NoError(t, os.WriteFile(certPath, newCert, 0o600))
		select {
		case <-notify:
		case <-time.After(10 * time.Second):
			require.Fail(t, "timeout waiting for certificate reload")
		}
	}
	close(stop)
	wg.Wait()
}
