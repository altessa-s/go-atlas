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
	if err == nil {
		t.Error("expected error for empty bucket")
	}
}

func TestNew_EmptyCertKey(t *testing.T) {
	_, err := tlss3.New("bucket", "", "key.key", "")
	if err == nil {
		t.Error("expected error for empty cert key")
	}
}

func TestNew_EmptyPrivKeyKey(t *testing.T) {
	_, err := tlss3.New("bucket", "cert.crt", "", "")
	if err == nil {
		t.Error("expected error for empty private key key")
	}
}

func TestNew_ValidCert(t *testing.T) {
	client, _, _ := newMockClient(t)

	p, err := tlss3.New("my-bucket", "certs/server.crt", "certs/server.key", "",
		tlss3.WithS3Client(client),
		tlss3.WithPollInterval(time.Hour),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer p.Close(t.Context())

	if p.Type() != tlsproviders.ProviderTypeS3 {
		t.Errorf("Type() = %v, want %v", p.Type(), tlsproviders.ProviderTypeS3)
	}
}

func TestS3_TLSConfig(t *testing.T) {
	client, _, _ := newMockClient(t)

	p, err := tlss3.New("my-bucket", "certs/server.crt", "certs/server.key", "",
		tlss3.WithS3Client(client),
		tlss3.WithPollInterval(time.Hour),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer p.Close(t.Context())

	config, err := p.TLSConfig()
	if err != nil {
		t.Fatalf("TLSConfig() error = %v", err)
	}
	if config == nil {
		t.Fatal("TLSConfig() returned nil")
	}
	if len(config.Certificates) == 0 {
		t.Error("TLSConfig() has no certificates")
	}
}

func TestS3_TLSConfig_ReturnsCopy(t *testing.T) {
	client, _, _ := newMockClient(t)

	p, err := tlss3.New("my-bucket", "certs/server.crt", "certs/server.key", "",
		tlss3.WithS3Client(client),
		tlss3.WithPollInterval(time.Hour),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer p.Close(t.Context())

	cfg1, _ := p.TLSConfig()
	cfg2, _ := p.TLSConfig()

	cfg1.ServerName = "modified"
	if cfg2.ServerName == "modified" {
		t.Error("TLSConfig() should return independent copies")
	}
}

func TestS3_PollDetectsETagChange(t *testing.T) {
	client, _, _ := newMockClient(t)
	notifyCh := make(chan struct{}, 2)

	p, err := tlss3.New("my-bucket", "certs/server.crt", "certs/server.key", "",
		tlss3.WithS3Client(client),
		tlss3.WithPollInterval(50*time.Millisecond),
		tlss3.WithReloadNotifyChan(notifyCh),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer p.Close(t.Context())

	// Drain initial load notification
	select {
	case <-notifyCh:
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for initial notification")
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
		t.Fatal("timeout waiting for reload notification")
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
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer p.Close(t.Context())

	// Drain initial notification
	select {
	case <-notifyCh:
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for initial notification")
	}

	// Wait a few poll intervals without any ETag changes
	time.Sleep(200 * time.Millisecond)

	// No additional notifications should have been sent
	select {
	case <-notifyCh:
		t.Error("unexpected reload notification when ETags haven't changed")
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
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer p.Close(t.Context())

	if algo := client.getSSEAlgorithm.Load(); algo == nil || *algo != "AES256" {
		t.Errorf("expected SSE algorithm AES256, got %v", algo)
	}

	if key := client.getSSEKey.Load(); key == nil || *key != "my-secret-key" {
		t.Errorf("expected SSE customer key %q, got %v", "my-secret-key", key)
	}
}

func TestS3_DownloadError(t *testing.T) {
	client := &mockS3Client{
		objects: map[string]*mockObject{},
	}

	_, err := tlss3.New("my-bucket", "certs/server.crt", "certs/server.key", "",
		tlss3.WithS3Client(client),
	)
	if err == nil {
		t.Error("expected error when objects don't exist")
	}
}

func TestS3_Close(t *testing.T) {
	client, _, _ := newMockClient(t)

	p, err := tlss3.New("my-bucket", "certs/server.crt", "certs/server.key", "",
		tlss3.WithS3Client(client),
		tlss3.WithPollInterval(time.Hour),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	if err := p.Close(ctx); err != nil {
		t.Errorf("Close() error = %v", err)
	}
}

func TestS3_Type(t *testing.T) {
	client, _, _ := newMockClient(t)

	p, err := tlss3.New("my-bucket", "certs/server.crt", "certs/server.key", "",
		tlss3.WithS3Client(client),
		tlss3.WithPollInterval(time.Hour),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer p.Close(t.Context())

	if got := p.Type(); got != tlsproviders.ProviderTypeS3 {
		t.Errorf("Type() = %v, want %v", got, tlsproviders.ProviderTypeS3)
	}
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
			if (err != nil) != tt.wantErr {
				t.Errorf("New() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
