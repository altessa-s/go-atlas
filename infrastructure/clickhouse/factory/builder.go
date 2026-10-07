// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"cmp"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"

	"github.com/altessa-s/go-atlas/observability/health"

	chgo "github.com/ClickHouse/clickhouse-go/v2"
	clickhouseconfig "github.com/altessa-s/go-atlas/config/clickhouse"
	coremaps "github.com/altessa-s/go-atlas/core/collections/maps"
	corefactory "github.com/altessa-s/go-atlas/core/factory"
	tlsfactory "github.com/altessa-s/go-atlas/security/tlsutils/factory"
)

// DefaultHealthServiceName is the name the health checker registers under
// when [ClickHouseBuilder.UseHealthServiceName] is not called.
const DefaultHealthServiceName = "clickhouse"

// ErrInvalidConnectionURI is returned when the configured connectionURI is
// not a valid ClickHouse DSN. The parse error is not included: it would quote
// the DSN, credentials included.
var ErrInvalidConnectionURI = errors.New("clickhouse: invalid connectionURI")

// compressionMethods maps the configuration enum onto the driver's
// compression methods. Built once and only read afterwards.
var compressionMethods = coremaps.NewImmutableMap(map[clickhouseconfig.Compression]chgo.CompressionMethod{
	clickhouseconfig.CompressionNone:    chgo.CompressionNone,
	clickhouseconfig.CompressionLZ4:     chgo.CompressionLZ4,
	clickhouseconfig.CompressionZSTD:    chgo.CompressionZSTD,
	clickhouseconfig.CompressionGZIP:    chgo.CompressionGZIP,
	clickhouseconfig.CompressionDeflate: chgo.CompressionDeflate,
	clickhouseconfig.CompressionBrotli:  chgo.CompressionBrotli,
})

// ClickHouseBuilder assembles a ClickHouse [driver.Conn] step by step using
// a fluent API. Create instances with [New]. Errors are accumulated and
// reported at [ClickHouseBuilder.Build] time. The builder is not safe for
// concurrent use.
type ClickHouseBuilder struct {
	corefactory.Base
	cfg  *clickhouseconfig.Config
	errs []error

	// Dependencies
	tlsConfig         *tls.Config
	healthCoordinator *health.Coordinator
	healthServiceName string
}

// New creates a [ClickHouseBuilder] for the given ClickHouse config.
// Config can be nil — the error surfaces at [ClickHouseBuilder.Build] time.
func New(cfg *clickhouseconfig.Config) *ClickHouseBuilder {
	return &ClickHouseBuilder{
		Base: corefactory.NewBase(slog.New(slog.DiscardHandler)),
		cfg:  cfg,
	}
}

// Build opens a ClickHouse connection from configuration.
//
// The driver connects lazily, so a successful Build does not prove the
// server is reachable; the registered health checker is what reports that.
// When a [health.Coordinator] was provided via
// [ClickHouseBuilder.UseHealthCoordinator], a checker is registered under
// [DefaultHealthServiceName] unless another name was set.
//
// The caller owns the returned connection and must Close it.
func (b *ClickHouseBuilder) Build(_ context.Context) (driver.Conn, error) {
	if err := corefactory.JoinErrors(b.errs); err != nil {
		return nil, err
	}

	if b.cfg == nil {
		return nil, fmt.Errorf("configuration is required")
	}

	opts, err := b.ClientOptions()
	if err != nil {
		return nil, err
	}

	conn, err := chgo.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("open clickhouse connection: %w", err)
	}

	if b.healthCoordinator != nil {
		b.healthCoordinator.RegisterService(
			cmp.Or(b.healthServiceName, DefaultHealthServiceName),
			&clickHouseHealthChecker{conn: conn, logger: b.Logger()},
		)
	}

	return conn, nil
}

// ClientOptions builds the driver [chgo.Options] from the builder's
// configuration. When [clickhouseconfig.Config.ConnectionURI] is set, the DSN is
// parsed first and the pool, timeout, compression, settings, and TLS fields
// are applied on top. Otherwise the options are built from the individual
// config fields.
func (b *ClickHouseBuilder) ClientOptions() (*chgo.Options, error) {
	var opts *chgo.Options
	if b.cfg.UseConnectionURI() {
		var err error
		if opts, err = b.optionsFromURI(); err != nil {
			return nil, err
		}
	} else {
		opts = b.optionsFromFields()
	}

	tlsCfg, err := b.resolveTLS()
	if err != nil {
		return nil, err
	}
	if tlsCfg != nil {
		opts.TLS = tlsCfg
	}

	return opts, nil
}

// resolveTLS returns the TLS configuration: the one injected with
// [ClickHouseBuilder.UseTlsConfig], otherwise one built from the `tls` block
// of the configuration, otherwise nil (plaintext, or whatever the DSN says).
func (b *ClickHouseBuilder) resolveTLS() (*tls.Config, error) {
	if b.tlsConfig != nil {
		return b.tlsConfig, nil
	}
	if b.cfg.TLS == nil {
		return nil, nil //nolint:nilnil // no TLS configured
	}
	tlsCfg, err := tlsfactory.New(nil).UseLogger(b.Logger()).CreateClientConfig(b.cfg.TLS)
	if err != nil {
		return nil, b.WrapError(err, "failed to build clickhouse TLS configuration")
	}
	return tlsCfg, nil
}

// optionsFromURI parses the DSN and layers the remaining config on top.
func (b *ClickHouseBuilder) optionsFromURI() (*chgo.Options, error) {
	opts, err := chgo.ParseDSN(b.cfg.ConnectionURI.Expose())
	if err != nil {
		// The driver's error quotes the DSN, credentials included; it must
		// not reach a log, so it is replaced rather than wrapped.
		return nil, ErrInvalidConnectionURI
	}

	b.applyCommon(opts)

	return opts, nil
}

// optionsFromFields builds the options from the individual config fields.
func (b *ClickHouseBuilder) optionsFromFields() *chgo.Options {
	opts := &chgo.Options{
		Addr: b.cfg.Hosts,
		Auth: chgo.Auth{
			Database: b.cfg.Database,
			Username: b.cfg.Username,
			Password: b.cfg.Password.Expose(),
		},
	}

	b.applyCommon(opts)

	return opts
}

// applyCommon applies the settings that are independent of how the
// connection target was described.
func (b *ClickHouseBuilder) applyCommon(opts *chgo.Options) {
	opts.DialTimeout = b.cfg.DialTimeout
	opts.ReadTimeout = b.cfg.ReadTimeout
	opts.MaxOpenConns = b.cfg.MaxOpenConns
	opts.MaxIdleConns = b.cfg.MaxIdleConns
	opts.ConnMaxLifetime = b.cfg.ConnMaxLifetime

	if method, ok := compressionMethods.Get(b.cfg.Compression); ok {
		opts.Compression = &chgo.Compression{Method: method}
	}

	if len(b.cfg.Settings) > 0 {
		settings := make(chgo.Settings, len(b.cfg.Settings))
		for k, v := range b.cfg.Settings {
			settings[k] = v
		}
		opts.Settings = settings
	}
}
