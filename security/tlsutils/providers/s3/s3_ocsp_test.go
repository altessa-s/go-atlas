// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package tlss3_test

import (
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"
	"github.com/altessa-s/go-atlas/security/tlsutils/providers/internal/staplertest"

	tlss3 "github.com/altessa-s/go-atlas/security/tlsutils/providers/s3"
)

// TestS3_OCSPStaplingAppliedOnce guards against the constructor wrapping an
// already-stapled config a second time: one handshake must hit the stapler once.
func TestS3_OCSPStaplingAppliedOnce(t *testing.T) {
	t.Parallel()
	client, _, _ := newMockClient(t)
	stapler := &staplertest.CountingStapler{}

	p, err := tlss3.New("my-bucket", "certs/server.crt", "certs/server.key", "",
		tlss3.WithS3Client(client),
		tlss3.WithOcspStapler(stapler),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = p.Close(t.Context()) })

	cfg, err := p.TLSConfig()
	require.NoError(t, err)
	require.Equal(t, staplertest.Staple, staplertest.Handshake(t, cfg))
	require.Equal(t, int32(1), stapler.Calls())
}

// TestS3_ReloadPublishesFullyStapledConfig reloads the certificate while
// readers clone the published config. Under -race this catches a config that
// is mutated after publication; every clone must carry the stapling callback.
func TestS3_ReloadPublishesFullyStapledConfig(t *testing.T) {
	t.Parallel()
	client, _, _ := newMockClient(t)
	notify := make(chan struct{}, 1)

	p, err := tlss3.New("my-bucket", "certs/server.crt", "certs/server.key", "",
		tlss3.WithS3Client(client),
		tlss3.WithOcspStapler(&staplertest.CountingStapler{}),
		tlss3.WithPollInterval(time.Millisecond),
		tlss3.WithReloadNotifyChan(notify),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = p.Close(t.Context()) })
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
				cfg, cerr := p.TLSConfig()
				if cerr != nil || cfg.GetCertificate == nil {
					t.Error("published TLS config lacks the OCSP stapling callback")
					return
				}
			}
		})
	}

	for i := range 5 {
		certPEM, keyPEM, _ := testhelpers.SelfSignedCert(t)
		gen := strconv.Itoa(i + 2)
		client.SetObject("certs/server.key", &mockObject{data: keyPEM, etag: `"etag-key-` + gen + `"`})
		client.SetObject("certs/server.crt", &mockObject{data: certPEM, etag: `"etag-cert-` + gen + `"`})
		select {
		case <-notify:
		case <-time.After(10 * time.Second):
			require.Fail(t, "timeout waiting for certificate reload")
		}
	}
	close(stop)
	wg.Wait()
}
