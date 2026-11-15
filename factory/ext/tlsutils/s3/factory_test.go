// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas-core/types/redacted"
	"github.com/altessa-s/go-atlas/factory/ext/tlsutils/s3"
	"github.com/altessa-s/go-atlas/internal/testhelpers"

	tlsconfig "github.com/altessa-s/go-atlas/config/tls"
	tlsfactory "github.com/altessa-s/go-atlas/factory/ext/tlsutils"
	tlsproviders "github.com/altessa-s/go-tlsutils/providers"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// fakeS3 serves objects from memory and records the SSE-C key it was sent.
type fakeS3 struct {
	mu      sync.Mutex
	objects map[string][]byte
	sseKey  atomic.Pointer[string]
}

func (f *fakeS3) GetObject(_ context.Context, in *s3.GetObjectInput, _ ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	if in.SSECustomerKey != nil {
		f.sseKey.Store(in.SSECustomerKey)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	data, ok := f.objects[*in.Key]
	if !ok {
		return nil, &s3types.NotFound{}
	}
	return &s3.GetObjectOutput{Body: io.NopCloser(bytes.NewReader(data)), ETag: aws.String("etag")}, nil
}

func (f *fakeS3) HeadObject(_ context.Context, in *s3.HeadObjectInput, _ ...func(*s3.Options)) (*s3.HeadObjectOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.objects[*in.Key]; !ok {
		return nil, &s3types.NotFound{}
	}
	return &s3.HeadObjectOutput{ETag: aws.String("etag")}, nil
}

func TestBuilder_SendsSSECustomerKey(t *testing.T) {
	t.Parallel()
	certPEM, keyPEM, _ := testhelpers.SelfSignedCert(t)
	s3c := &fakeS3{objects: map[string][]byte{"tls/cert.pem": certPEM, "tls/key.pem": keyPEM}}

	providers, err := tlsfactory.New(&tlsconfig.Provider{S3: &tlsconfig.ProviderS3{
		Bucket: "certs", CertificateKey: "tls/cert.pem", PrivateKeyKey: "tls/key.pem",
		SSE: &tlsconfig.ProviderS3SSE{Type: tlsconfig.SSETypeConfigC, CustomerKey: redacted.RedactedString("k3y"), CustomerKeyMD5: "md5"},
	}}).
		UseLogger(slog.New(slog.DiscardHandler)).
		UseProvider(tlsproviders.ProviderTypeS3, factory.Builder(s3c)).
		Build()
	require.NoError(t, err)
	t.Cleanup(func() {
		providers.Close(context.Background(), func(_ tlsproviders.Provider, err error) {
			t.Errorf("close provider: %v", err)
		})
	})

	p, ok := providers.Get(tlsproviders.ProviderTypeS3)
	require.True(t, ok)
	require.Equal(t, tlsproviders.ProviderTypeS3, p.Type())
	require.NotNil(t, s3c.sseKey.Load(), "SSE-C key must be sent to S3")
	require.Equal(t, "k3y", *s3c.sseKey.Load())
}

func TestBuilder_MissingObjectsFail(t *testing.T) {
	t.Parallel()
	s3c := &fakeS3{objects: map[string][]byte{}}

	_, err := factory.Builder(s3c)(&tlsconfig.Provider{S3: &tlsconfig.ProviderS3{
		Bucket: "certs", CertificateKey: "c", PrivateKeyKey: "k",
		SSE: &tlsconfig.ProviderS3SSE{Type: tlsconfig.SSETypeConfigKMS},
	}}, tlsfactory.ProviderEnv{})
	require.Error(t, err)
}

// Without a client the builder creates one from the s3 section and talks to
// the configured endpoint.
func TestBuilder_ClientFromConfig(t *testing.T) {
	t.Parallel()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)

	_, err := factory.Builder(nil)(&tlsconfig.Provider{S3: &tlsconfig.ProviderS3{
		Endpoint: srv.URL, Region: "us-east-1", PathStyle: true,
		AccessKey: redacted.RedactedString("ak"), SecretKey: redacted.RedactedString("sk"),
		Bucket: "certs", CertificateKey: "c", PrivateKeyKey: "k",
	}}, tlsfactory.ProviderEnv{})
	require.Error(t, err)
	require.Positive(t, hits.Load(), "the configured endpoint must be used")
}
