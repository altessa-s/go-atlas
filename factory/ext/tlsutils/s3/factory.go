// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/altessa-s/go-atlas-core/collections/slices"

	tlsconfig "github.com/altessa-s/go-atlas/config/tls"
	tlsfactory "github.com/altessa-s/go-atlas/factory/ext/tlsutils"
	tlsproviders "github.com/altessa-s/go-tlsutils/providers"
	tlss3 "github.com/altessa-s/go-tlsutils/providers/s3"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
)

// Builder returns the provider builder of the s3 type, ready for
// tlsfactory.ProvidersBuilder.UseProvider. The provider reads the certificate
// and key objects configured by the s3 section through client; a nil client
// is built from the section (endpoint, region, static credentials).
func Builder(client tlss3.S3API) tlsfactory.ProviderBuilder {
	return func(cfg *tlsconfig.Provider, env tlsfactory.ProviderEnv) (tlsproviders.Provider, error) {
		s := cfg.S3
		c := client
		if c == nil {
			var err error
			if c, err = newClient(s); err != nil {
				return nil, err
			}
		}

		opts := []tlss3.Option{
			tlss3.WithS3Client(c),
			tlss3.WithPollInterval(s.PollInterval),
		}
		if s.SSE != nil {
			opts = append(opts, tlss3.WithSseType(tlss3.SSEType(s.SSE.Type)))
			switch s.SSE.Type {
			case tlsconfig.SSETypeConfigC:
				opts = append(opts, tlss3.WithSseCustomerKey(s.SSE.CustomerKey.Expose()))
				opts = append(opts, tlss3.WithSseCustomerKeyMD5(s.SSE.CustomerKeyMD5))
			case tlsconfig.SSETypeConfigS3, tlsconfig.SSETypeConfigKMS:
				// SSE-S3 and SSE-KMS are transparent on read — S3 decrypts
				// server-side (KMS needs kms:Decrypt on the reader's role),
				// so no additional options are needed.
			}
		}
		opts = slices.AppendIf(opts, env.Logger != nil, tlss3.WithLogger(env.Logger))
		opts = slices.AppendIf(opts, env.OCSPStapler != nil, tlss3.WithOcspStapler(env.OCSPStapler))

		p, err := tlss3.New(s.Bucket, s.CertificateKey, s.PrivateKeyKey, s.PrivateKeyPassword.Expose(), opts...)
		if err != nil {
			return nil, err
		}
		return p, nil
	}
}

// newClient builds an S3 client from the s3 section.
func newClient(cfg *tlsconfig.ProviderS3) (tlss3.S3API, error) {
	awsCfg, err := awsconfig.LoadDefaultConfig(context.Background(),
		awsconfig.WithRegion(cfg.Region),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			cfg.AccessKey.Expose(),
			cfg.SecretKey.Expose(),
			"",
		)),
	)
	if err != nil {
		return nil, err
	}

	return s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.BaseEndpoint = &cfg.Endpoint
		o.UsePathStyle = cfg.PathStyle
	}), nil
}
