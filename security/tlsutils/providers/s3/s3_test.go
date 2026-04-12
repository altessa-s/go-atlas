// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package tlss3_test

import (
	"bytes"
	"context"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"

	tlsproviders "github.com/altessa-s/go-atlas/security/tlsutils/providers"
	tlss3 "github.com/altessa-s/go-atlas/security/tlsutils/providers/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// mockS3Client implements tlss3.S3API for testing.
// Thread-safe: the poll goroutine reads objects while tests may update them.
type mockS3Client struct {
	mu      sync.RWMutex
	objects map[string]*mockObject // key -> object
	headErr error
	getErr  error
}

type mockObject struct {
	data []byte
	etag string
}

func (m *mockS3Client) SetObject(key string, obj *mockObject) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.objects[key] = obj
}

func (m *mockS3Client) GetObject(_ context.Context, input *s3.GetObjectInput, _ ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.getErr != nil {
		return nil, m.getErr
	}
	obj, ok := m.objects[*input.Key]
	if !ok {
		return nil, &s3types.NotFound{}
	}
	return &s3.GetObjectOutput{
		Body: io.NopCloser(bytes.NewReader(obj.data)),
		ETag: aws.String(obj.etag),
	}, nil
}

func (m *mockS3Client) HeadObject(_ context.Context, input *s3.HeadObjectInput, _ ...func(*s3.Options)) (*s3.HeadObjectOutput, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.headErr != nil {
		return nil, m.headErr
	}
	obj, ok := m.objects[*input.Key]
	if !ok {
		return nil, &s3types.NotFound{}
	}
	return &s3.HeadObjectOutput{
		ETag: aws.String(obj.etag),
	}, nil
}

// sseCapturingClient captures SSE-C parameters passed to GetObject and HeadObject.
type sseCapturingClient struct {
	mockS3Client
	getSSEAlgorithm  atomic.Pointer[string]
	getSSEKey        atomic.Pointer[string]
	headSSEAlgorithm atomic.Pointer[string]
	headSSEKey       atomic.Pointer[string]
}

func (m *sseCapturingClient) GetObject(ctx context.Context, input *s3.GetObjectInput, opts ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	if input.SSECustomerAlgorithm != nil {
		m.getSSEAlgorithm.Store(input.SSECustomerAlgorithm)
	}
	if input.SSECustomerKey != nil {
		m.getSSEKey.Store(input.SSECustomerKey)
	}
	return m.mockS3Client.GetObject(ctx, input, opts...)
}

func (m *sseCapturingClient) HeadObject(ctx context.Context, input *s3.HeadObjectInput, opts ...func(*s3.Options)) (*s3.HeadObjectOutput, error) {
	if input.SSECustomerAlgorithm != nil {
		m.headSSEAlgorithm.Store(input.SSECustomerAlgorithm)
	}
	if input.SSECustomerKey != nil {
		m.headSSEKey.Store(input.SSECustomerKey)
	}
	return m.mockS3Client.HeadObject(ctx, input, opts...)
}

func newMockClient(tb testing.TB) (*mockS3Client, []byte, []byte) {
	tb.Helper()
	certPEM, keyPEM, _ := testhelpers.SelfSignedCert(tb)
	client := &mockS3Client{
		objects: map[string]*mockObject{
			"certs/server.crt": {data: certPEM, etag: `"etag-cert-1"`},
			"certs/server.key": {data: keyPEM, etag: `"etag-key-1"`},
		},
	}
	return client, certPEM, keyPEM
}

func TestNew_EmptyBucket(t *testing.T) {
	_, err := tlss3.New("", "cert.crt", "key.key", "")
	require.Error(t, err)
}

func TestNew_EmptyCertKey(t *testing.T) {
	_, err := tlss3.New("bucket", "", "key.key", "")
	require.Error(t, err)
}

func TestNew_EmptyPrivKeyKey(t *testing.T) {
	_, err := tlss3.New("bucket", "cert.crt", "", "")
	require.Error(t, err)
}

func TestNew_ValidCert(t *testing.T) {
	client, _, _ := newMockClient(t)

	p, err := tlss3.New("my-bucket", "certs/server.crt", "certs/server.key", "",
		tlss3.WithS3Client(client),
		tlss3.WithPollInterval(time.Hour),
	)
	require.NoError(t, err)
	defer p.Close(t.Context())

	require.Equal(t, tlsproviders.ProviderTypeS3, p.Type())
}

func TestS3_TLSConfig(t *testing.T) {
	client, _, _ := newMockClient(t)

	p, err := tlss3.New("my-bucket", "certs/server.crt", "certs/server.key", "",
		tlss3.WithS3Client(client),
		tlss3.WithPollInterval(time.Hour),
	)
	require.NoError(t, err)
	defer p.Close(t.Context())

	config, err := p.TLSConfig()
	require.NoError(t, err)
	require.NotNil(t, config)
	require.NotEmpty(t, config.Certificates)
}

func TestS3_TLSConfig_ReturnsCopy(t *testing.T) {
	client, _, _ := newMockClient(t)

	p, err := tlss3.New("my-bucket", "certs/server.crt", "certs/server.key", "",
		tlss3.WithS3Client(client),
		tlss3.WithPollInterval(time.Hour),
	)
	require.NoError(t, err)
	defer p.Close(t.Context())

	cfg1, _ := p.TLSConfig()
	cfg2, _ := p.TLSConfig()

	cfg1.ServerName = "modified"
	require.NotEqual(t, "modified", cfg2.ServerName)
}

func TestS3_PollDetectsETagChange(t *testing.T) {
	client, _, _ := newMockClient(t)
	notifyCh := make(chan struct{}, 2)

	p, err := tlss3.New("my-bucket", "certs/server.crt", "certs/server.key", "",
		tlss3.WithS3Client(client),
		tlss3.WithPollInterval(50*time.Millisecond),
		tlss3.WithReloadNotifyChan(notifyCh),
	)
	require.NoError(t, err)
	defer p.Close(t.Context())

	// Drain initial load notification
	select {
	case <-notifyCh:
	case <-time.After(time.Second):
		require.Fail(t, "timeout waiting for initial notification")
	}

	// Update cert PEM data with new cert and change ETags
	certPEM2, keyPEM2, _ := testhelpers.SelfSignedCert(t)
	client.SetObject("certs/server.crt", &mockObject{data: certPEM2, etag: `"etag-cert-2"`})
	client.SetObject("certs/server.key", &mockObject{data: keyPEM2, etag: `"etag-key-2"`})

	// Wait for reload notification
	select {
	case <-notifyCh:
		// success - poll detected change
	case <-time.After(5 * time.Second):
		require.Fail(t, "timeout waiting for reload notification")
	}
}

func TestS3_PollSameETag_NoReload(t *testing.T) {
	client, _, _ := newMockClient(t)
	notifyCh := make(chan struct{}, 10)

	p, err := tlss3.New("my-bucket", "certs/server.crt", "certs/server.key", "",
		tlss3.WithS3Client(client),
		tlss3.WithPollInterval(50*time.Millisecond),
		tlss3.WithReloadNotifyChan(notifyCh),
	)
	require.NoError(t, err)
	defer p.Close(t.Context())

	// Drain initial notification
	select {
	case <-notifyCh:
	case <-time.After(time.Second):
		require.Fail(t, "timeout waiting for initial notification")
	}

	// Wait a few poll intervals without any ETag changes
	time.Sleep(200 * time.Millisecond)

	// No additional notifications should have been sent
	select {
	case <-notifyCh:
		require.Fail(t, "unexpected reload notification when ETags haven't changed")
	default:
		// expected
	}
}

func TestS3_SSEC_ParamsPassedToGetObject(t *testing.T) {
	certPEM, keyPEM, _ := testhelpers.SelfSignedCert(t)
	client := &sseCapturingClient{
		mockS3Client: mockS3Client{
			objects: map[string]*mockObject{
				"certs/server.crt": {data: certPEM, etag: `"etag1"`},
				"certs/server.key": {data: keyPEM, etag: `"etag2"`},
			},
		},
	}

	p, err := tlss3.New("my-bucket", "certs/server.crt", "certs/server.key", "",
		tlss3.WithS3Client(client),
		tlss3.WithPollInterval(time.Hour),
		tlss3.WithSseType(tlss3.SSETypeC),
		tlss3.WithSseCustomerKey("my-secret-key"),
		tlss3.WithSseCustomerKeyMD5("my-key-md5"),
	)
	require.NoError(t, err)
	defer p.Close(t.Context())

	algo := client.getSSEAlgorithm.Load()
	require.NotNil(t, algo)
	require.Equal(t, "AES256", *algo)

	key := client.getSSEKey.Load()
	require.NotNil(t, key)
	require.Equal(t, "my-secret-key", *key)
}

func TestS3_DownloadError(t *testing.T) {
	client := &mockS3Client{
		objects: map[string]*mockObject{},
	}

	_, err := tlss3.New("my-bucket", "certs/server.crt", "certs/server.key", "",
		tlss3.WithS3Client(client),
	)
	require.Error(t, err)
}

func TestS3_Close(t *testing.T) {
	client, _, _ := newMockClient(t)

	p, err := tlss3.New("my-bucket", "certs/server.crt", "certs/server.key", "",
		tlss3.WithS3Client(client),
		tlss3.WithPollInterval(time.Hour),
	)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	require.NoError(t, p.Close(ctx))
}

func TestS3_Type(t *testing.T) {
	client, _, _ := newMockClient(t)

	p, err := tlss3.New("my-bucket", "certs/server.crt", "certs/server.key", "",
		tlss3.WithS3Client(client),
		tlss3.WithPollInterval(time.Hour),
	)
	require.NoError(t, err)
	defer p.Close(t.Context())

	require.Equal(t, tlsproviders.ProviderTypeS3, p.Type())
}

func TestNew_InvalidParams(t *testing.T) {
	client, _, _ := newMockClient(t)

	tests := []struct {
		name       string
		bucket     string
		certKey    string
		privKeyKey string
		opts       []tlss3.Option
		wantErr    bool
	}{
		{"all empty", "", "", "", nil, true},
		{"bucket empty", "", "cert.crt", "key.key", []tlss3.Option{tlss3.WithS3Client(client)}, true},
		{"cert empty", "bucket", "", "key.key", []tlss3.Option{tlss3.WithS3Client(client)}, true},
		{"key empty", "bucket", "cert.crt", "", []tlss3.Option{tlss3.WithS3Client(client)}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tlss3.New(tt.bucket, tt.certKey, tt.privKeyKey, "", tt.opts...)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
