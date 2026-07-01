// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"

	"github.com/aws/aws-sdk-go-v2/credentials"

	"github.com/altessa-s/go-atlas/auth/opa"
	"github.com/altessa-s/go-atlas/auth/opa/sources/filesystem"
	"github.com/altessa-s/go-atlas/auth/opa/sources/gitlab"
	"github.com/altessa-s/go-atlas/config"
	"github.com/altessa-s/go-atlas/core/collections/slices"
	"github.com/altessa-s/go-atlas/observability/health"

	embedsrc "github.com/altessa-s/go-atlas/auth/opa/sources/embed"
	s3source "github.com/altessa-s/go-atlas/auth/opa/sources/s3"
	corefactory "github.com/altessa-s/go-atlas/core/factory"
	corescheduler "github.com/altessa-s/go-atlas/core/scheduler"
	httpclient "github.com/altessa-s/go-atlas/transport/http/client"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
)

// ManagerBuilder assembles an [opa.Manager] from configuration and
// injected dependencies using a fluent API with deferred error accumulation.
type ManagerBuilder struct {
	corefactory.Base
	cfg  *config.OPA
	errs []error

	// Dependencies (set via Use*).
	scheduler         corescheduler.TaskRegistrar
	healthCoordinator *health.Coordinator
	embedFS           fs.FS
	embedDir          string
	s3Client          s3source.S3API
}

// New creates a new [ManagerBuilder] for the given OPA config.
// A nil cfg is accepted; the error surfaces at [ManagerBuilder.Build] time.
func New(cfg *config.OPA) *ManagerBuilder {
	return &ManagerBuilder{
		Base: corefactory.NewBase(slog.New(slog.DiscardHandler)),
		cfg:  cfg,
	}
}

// Build assembles and returns the OPA manager. All errors accumulated
// during the fluent chain are returned here.
func (b *ManagerBuilder) Build(ctx context.Context) (*opa.Manager, error) {
	if err := corefactory.JoinErrors(b.errs); err != nil {
		return nil, err
	}

	if b.cfg == nil {
		return nil, fmt.Errorf("configuration is required")
	}

	if !b.cfg.IsEnabled() {
		return nil, b.WrapError(fmt.Errorf("OPA not enabled"), "configuration check failed")
	}

	source, err := b.buildSource(ctx)
	if err != nil {
		return nil, err
	}

	manager, err := b.buildManager(ctx, source)
	if err != nil {
		_ = source.Close()
		return nil, err
	}

	if b.cfg.WatchBundle {
		if err = manager.StartWatching(ctx); err != nil {
			_ = manager.Close()
			return nil, b.WrapError(err, "failed to start watching")
		}
	}

	return manager, nil
}

// buildSource creates a PolicySource from config based on the configured provider.
func (b *ManagerBuilder) buildSource(ctx context.Context) (opa.PolicySource, error) {
	switch b.cfg.Source {
	case config.OPASourceGitLab:
		return b.buildGitLabSource()
	case config.OPASourceEmbed:
		return b.buildEmbedSource()
	case config.OPASourceS3:
		return b.buildS3Source(ctx)
	default:
		return b.buildFilesystemSource()
	}
}

// buildFilesystemSource creates a filesystem PolicySource from config.
func (b *ManagerBuilder) buildFilesystemSource() (opa.PolicySource, error) {
	cfg := b.cfg

	fsOpts := []filesystem.Option{
		filesystem.WithLogger(b.Logger()),
		filesystem.WithExtensions(cfg.FileExtensions...),
	}
	fsOpts = slices.AppendIf(fsOpts, cfg.IncludeData, filesystem.WithIncludeData())

	source, err := filesystem.New(cfg.BundlePath, fsOpts...)
	if err != nil {
		return nil, b.WrapError(err, "failed to create filesystem policy source")
	}
	return source, nil
}

// buildGitLabSource creates a GitLab PolicySource from config.
func (b *ManagerBuilder) buildGitLabSource() (opa.PolicySource, error) {
	gl := b.cfg.GitLab

	opts := []gitlab.Option{
		gitlab.WithEndpoint(gl.Endpoint),
		gitlab.WithToken(gl.Token.Expose()),
		gitlab.WithProjectID(gl.ProjectID),
		gitlab.WithLogger(b.Logger()),
	}

	opts = slices.AppendIf(opts, gl.Ref != "", gitlab.WithRef(gl.Ref))

	opts = slices.AppendIf(opts, gl.Dir != "", gitlab.WithDir(gl.Dir))

	proxyOpts, err := gl.Proxy.ClientOptions()
	if err != nil {
		return nil, b.WrapError(err, "failed to materialize gitlab proxy options")
	}
	opts = slices.AppendIf(opts, len(proxyOpts) > 0, gitlab.WithHTTPClientOptions(proxyOpts...))

	opts = slices.AppendIf(opts, b.cfg.IncludeData, gitlab.WithIncludeData())

	source, err := gitlab.New(opts...)
	if err != nil {
		return nil, b.WrapError(err, "failed to create gitlab policy source")
	}
	return source, nil
}

// buildEmbedSource creates an embed PolicySource from config and injected fs.FS.
func (b *ManagerBuilder) buildEmbedSource() (opa.PolicySource, error) {
	if b.embedFS == nil {
		return nil, b.WrapError(fmt.Errorf("embed fs.FS is required for embed source"), "configuration check failed")
	}

	cfg := b.cfg

	embedOpts := []embedsrc.Option{
		embedsrc.WithLogger(b.Logger()),
		embedsrc.WithExtensions(cfg.FileExtensions...),
	}
	embedOpts = slices.AppendIf(embedOpts, cfg.IncludeData, embedsrc.WithIncludeData())

	source, err := embedsrc.New(b.embedFS, b.embedDir, embedOpts...)
	if err != nil {
		return nil, b.WrapError(err, "failed to create embed policy source")
	}
	return source, nil
}

// buildS3Source creates an S3 PolicySource from config and optionally injected client.
func (b *ManagerBuilder) buildS3Source(ctx context.Context) (opa.PolicySource, error) {
	s3Cfg := b.cfg.S3
	if s3Cfg == nil {
		return nil, b.WrapError(fmt.Errorf("S3 configuration is required for s3 source"), "configuration check failed")
	}

	client, err := b.resolveS3Client(ctx, s3Cfg)
	if err != nil {
		return nil, b.WrapError(err, "failed to create S3 client")
	}

	cfg := b.cfg

	s3Opts := []s3source.Option{
		s3source.WithS3Client(client),
		s3source.WithBucket(s3Cfg.Bucket),
		s3source.WithLogger(b.Logger()),
		s3source.WithExtensions(cfg.FileExtensions...),
	}

	s3Opts = slices.AppendIf(s3Opts, s3Cfg.Prefix != "", s3source.WithPrefix(s3Cfg.Prefix))

	s3Opts = slices.AppendIf(s3Opts, cfg.IncludeData, s3source.WithIncludeData())

	source, err := s3source.New(s3Opts...)
	if err != nil {
		return nil, b.WrapError(err, "failed to create s3 policy source")
	}
	return source, nil
}

// resolveS3Client returns the injected S3 client or creates one from config.
func (b *ManagerBuilder) resolveS3Client(ctx context.Context, s3Cfg *config.OPAS3) (s3source.S3API, error) {
	if b.s3Client != nil {
		return b.s3Client, nil
	}

	awsCfgOpts := []func(*awsconfig.LoadOptions) error{}

	if s3Cfg.Region != "" {
		awsCfgOpts = append(awsCfgOpts, awsconfig.WithRegion(s3Cfg.Region))
	}

	if s3Cfg.AccessKey.Expose() != "" && s3Cfg.SecretKey.Expose() != "" {
		awsCfgOpts = append(awsCfgOpts, awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(
				s3Cfg.AccessKey.Expose(),
				s3Cfg.SecretKey.Expose(),
				"",
			),
		))
	}

	// Wire go-atlas's resilient HTTP client only when proxy is
	// explicitly configured. Without an override the AWS SDK keeps its
	// own HTTP client (which already honors HTTP_PROXY/HTTPS_PROXY/
	// NO_PROXY env vars) and its own retry layer, avoiding double-retry
	// with our breaker / retry middleware.
	proxyOpts, err := s3Cfg.Proxy.ClientOptions()
	if err != nil {
		return nil, b.WrapError(err, "failed to materialize s3 proxy options")
	}
	if len(proxyOpts) > 0 {
		awsCfgOpts = append(awsCfgOpts, awsconfig.WithHTTPClient(httpclient.New(proxyOpts...)))
	}

	cfg, err := awsconfig.LoadDefaultConfig(ctx, awsCfgOpts...)
	if err != nil {
		return nil, fmt.Errorf("load AWS config: %w", err)
	}

	var s3Opts []func(*awss3.Options)
	if s3Cfg.Endpoint != "" {
		s3Opts = append(s3Opts, func(o *awss3.Options) {
			o.BaseEndpoint = &s3Cfg.Endpoint
		})
	}

	if s3Cfg.PathStyle {
		s3Opts = append(s3Opts, func(o *awss3.Options) {
			o.UsePathStyle = true
		})
	}

	return awss3.NewFromConfig(cfg, s3Opts...), nil
}

// buildManager creates the OPA Manager from source and config.
func (b *ManagerBuilder) buildManager(ctx context.Context, source opa.PolicySource) (*opa.Manager, error) {
	cfg := b.cfg

	managerOpts := []opa.Option{
		opa.WithLogger(b.Logger()),
		opa.WithHealthCoordinator(b.healthCoordinator),
	}
	managerOpts = slices.AppendIf(managerOpts, cfg.DecisionLogging, opa.WithDecisionLogging())

	managerOpts = slices.AppendIf(managerOpts, cfg.PollInterval > 0, opa.WithPollInterval(cfg.PollInterval))

	managerOpts = slices.AppendIf(managerOpts, b.scheduler != nil && cfg.UpdateSchedule != "",
		opa.WithScheduler(b.scheduler),
		opa.WithUpdateSchedule(cfg.UpdateSchedule, cfg.RunOnStart),
	)

	manager, err := opa.NewManager(ctx, source, cfg.Query, managerOpts...)
	if err != nil {
		return nil, b.WrapError(err, "failed to create manager")
	}
	return manager, nil
}
