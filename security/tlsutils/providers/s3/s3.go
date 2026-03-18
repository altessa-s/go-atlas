// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package tlss3

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/altessa-s/go-atlas/security/tlsutils"
	"github.com/altessa-s/go-atlas/security/tlsutils/ocsp"

	corecontext "github.com/altessa-s/go-atlas/core/context"
	tlsproviders "github.com/altessa-s/go-atlas/security/tlsutils/providers"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
)

const defaultPollInterval = 5 * time.Minute

// S3 represents an S3-based TLS provider with automatic certificate polling.
// Use New to create a new instance.
type S3 struct {
	bucket      string
	certKey     string
	privKeyKey  string
	keyPassword string

	mu          sync.RWMutex
	tlsConfig   *tls.Config
	certificate *tls.Certificate
	certETag    string
	keyETag     string

	ctx      context.Context
	cancel   context.CancelFunc
	pollDone chan struct{}

	options     *options
	ocspStapler tlsutils.OCSPStapler
	logger      *slog.Logger
}

// New creates a new S3-based TLS provider that downloads certificates from S3.
// The provider polls for changes using ETags and reloads certificates when they change.
// If no S3 client is provided via WithS3Client, a default client is created
// using the standard AWS credential chain (environment variables, shared config, IAM role, etc.).
//
// Example:
//
//	provider, err := tlss3.New("my-bucket", "certs/server.crt", "certs/server.key", "",
//		tlss3.WithS3Client(s3Client),
//		tlss3.WithPollInterval(5 * time.Minute),
//	)
//	if err != nil {
//		log.Fatal(err)
//	}
//	defer provider.Close(ctx)
func New(bucket, certObjectKey, privKeyObjectKey, keyPassword string, opts ...Option) (*S3, error) {
	if bucket == "" {
		return nil, errors.New("invalid bucket: name cannot be empty")
	}

	if certObjectKey == "" {
		return nil, errors.New("invalid certificate object key: key cannot be empty")
	}

	if privKeyObjectKey == "" {
		return nil, errors.New("invalid private key object key: key cannot be empty")
	}

	o := newOptions(opts...)

	if o.s3Client == nil {
		client, err := defaultS3Client(corecontext.OrBackground(o.ctx))
		if err != nil {
			return nil, fmt.Errorf("creating default S3 client: %w", err)
		}
		o.s3Client = client
	}

	if o.pollInterval <= 0 {
		o.pollInterval = defaultPollInterval
	}

	p := &S3{
		bucket:      bucket,
		certKey:     certObjectKey,
		privKeyKey:  privKeyObjectKey,
		keyPassword: keyPassword,
		pollDone:    make(chan struct{}),
		options:     o,
		ocspStapler: o.ocspStapler,
		logger:      o.logger,
	}

	baseCtx := corecontext.OrBackground(o.ctx)
	p.ctx, p.cancel = context.WithCancel(baseCtx)

	// Load initial certificate
	if err := p.loadCertificate(p.ctx); err != nil {
		p.cancel()
		return nil, err
	}

	// Initialize OCSP stapler if provided
	if p.ocspStapler != nil {
		if err := ocsp.StapleOCSPToConfig(p.tlsConfig, p.ocspStapler); err != nil {
			p.cancel()
			return nil, err
		}
	}

	// Start polling goroutine
	go p.pollForChanges()

	return p, nil
}

// Type returns the provider type identifier.
// Always returns ProviderTypeS3 for S3-based providers.
//
// Example:
//
//	fmt.Println(provider.Type()) // "s3"
func (p *S3) Type() tlsproviders.ProviderType {
	return tlsproviders.ProviderTypeS3
}

// TLSConfig returns the current TLS configuration with loaded certificates.
// The configuration is cloned to prevent external modifications.
//
// Example:
//
//	config, err := provider.TLSConfig()
//	server := &http.Server{TLSConfig: config}
func (p *S3) TLSConfig() (*tls.Config, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.tlsConfig == nil {
		return nil, errors.New("TLS config not loaded")
	}

	return p.tlsConfig.Clone(), nil
}

// Close stops the polling goroutine and releases all resources.
// The context controls the graceful shutdown timeout.
//
// Example:
//
//	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
//	defer cancel()
//	provider.Close(ctx)
func (p *S3) Close(ctx context.Context) error {
	p.cancel()

	ctx = corecontext.OrBackground(ctx)

	select {
	case <-p.pollDone:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (p *S3) loadCertificate(ctx context.Context) error {
	certData, certETag, err := p.downloadObject(ctx, p.bucket, p.certKey)
	if err != nil {
		return fmt.Errorf("downloading certificate %s/%s: %w", p.bucket, p.certKey, err)
	}

	keyData, keyETag, err := p.downloadObject(ctx, p.bucket, p.privKeyKey)
	if err != nil {
		return fmt.Errorf("downloading private key %s/%s: %w", p.bucket, p.privKeyKey, err)
	}

	cert, err := tlsutils.LoadFromBytes(certData, keyData, p.keyPassword)
	if err != nil {
		return fmt.Errorf("parsing certificate: %w", err)
	}

	tlsConfig := tlsutils.DefaultTLSConfig()
	tlsConfig.Certificates = []tls.Certificate{*cert}

	p.mu.Lock()
	p.tlsConfig = tlsConfig
	p.certificate = cert
	p.certETag = certETag
	p.keyETag = keyETag
	p.mu.Unlock()

	// If OCSP stapler exists, refresh OCSP for new certificate
	if p.ocspStapler != nil {
		//nolint:contextcheck // closures obtain context from TLS handshake info, not from caller
		if err := ocsp.StapleOCSPToConfig(tlsConfig, p.ocspStapler); err != nil {
			p.logger.WarnContext(ctx, "failed to apply OCSP stapling to reloaded certificate", slog.Any("error", err))
		}
	}

	p.logger.DebugContext(ctx, "certificate loaded from S3",
		"bucket", p.bucket,
		"cert_key", p.certKey,
		"key_key", p.privKeyKey,
		"ocsp_enabled", p.ocspStapler != nil)

	// Notify about reload if channel is set
	if p.options.reloadNotifyChan != nil {
		select {
		case p.options.reloadNotifyChan <- struct{}{}:
		default:
		}
	}

	return nil
}

func (p *S3) downloadObject(ctx context.Context, bucket, key string) ([]byte, string, error) {
	input := &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	}
	p.applySSEC(&input.SSECustomerAlgorithm, &input.SSECustomerKey, &input.SSECustomerKeyMD5)

	output, err := p.options.s3Client.GetObject(ctx, input)
	if err != nil {
		return nil, "", err
	}
	defer func() { _ = output.Body.Close() }()

	data, err := io.ReadAll(output.Body)
	if err != nil {
		return nil, "", err
	}

	return data, aws.ToString(output.ETag), nil
}

func (p *S3) checkForChanges(ctx context.Context) {
	certETag, err := p.headObjectETag(ctx, p.bucket, p.certKey)
	if err != nil {
		p.logger.WarnContext(ctx, "failed to check certificate ETag",
			"bucket", p.bucket, "key", p.certKey, slog.Any("error", err))
		return
	}

	keyETag, err := p.headObjectETag(ctx, p.bucket, p.privKeyKey)
	if err != nil {
		p.logger.WarnContext(ctx, "failed to check private key ETag",
			"bucket", p.bucket, "key", p.privKeyKey, slog.Any("error", err))
		return
	}

	p.mu.RLock()
	changed := certETag != p.certETag || keyETag != p.keyETag
	p.mu.RUnlock()

	if !changed {
		return
	}

	p.logger.InfoContext(ctx, "certificate change detected in S3, reloading",
		"bucket", p.bucket,
		"cert_key", p.certKey,
		"key_key", p.privKeyKey)

	if err := p.loadCertificate(ctx); err != nil {
		p.logger.ErrorContext(ctx, "failed to reload certificate from S3", slog.Any("error", err))
	}
}

func (p *S3) headObjectETag(ctx context.Context, bucket, key string) (string, error) {
	input := &s3.HeadObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	}
	p.applySSEC(&input.SSECustomerAlgorithm, &input.SSECustomerKey, &input.SSECustomerKeyMD5)

	output, err := p.options.s3Client.HeadObject(ctx, input)
	if err != nil {
		return "", err
	}

	return aws.ToString(output.ETag), nil
}

// applySSEC sets SSE-C encryption parameters when the provider is configured for
// customer-provided encryption. SSE-S3 and SSE-KMS are transparent on read operations.
func (p *S3) applySSEC(algorithm, key, keyMD5 **string) {
	if p.options.sseType != SSETypeC {
		return
	}
	*algorithm = aws.String("AES256")
	*key = aws.String(p.options.sseCustomerKey)
	*keyMD5 = aws.String(p.options.sseCustomerKeyMD5)
}

func (p *S3) pollForChanges() {
	defer close(p.pollDone)

	ticker := time.NewTicker(p.options.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-p.ctx.Done():
			return
		case <-ticker.C:
			p.checkForChanges(p.ctx)
		}
	}
}

func defaultS3Client(ctx context.Context) (*s3.Client, error) {
	cfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, err
	}
	return s3.NewFromConfig(cfg), nil
}

var _ tlsproviders.Provider = (*S3)(nil)
