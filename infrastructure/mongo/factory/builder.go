// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"cmp"
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"

	"github.com/altessa-s/go-atlas/config"
	"github.com/altessa-s/go-atlas/core/collections/slices"
	"github.com/altessa-s/go-atlas/data/mongo"
	"github.com/altessa-s/go-atlas/data/mongo/kms"
	"github.com/altessa-s/go-atlas/domain/converter"
	"github.com/altessa-s/go-atlas/observability/health"
	"github.com/altessa-s/go-atlas/observability/metrics"

	corefactory "github.com/altessa-s/go-atlas/core/factory"
	mongoOptions "go.mongodb.org/mongo-driver/v2/mongo/options"
)

// MongoBuilder assembles a [mongo.Mongo] wrapper step by step using a fluent API.
// Create instances with [New]. Errors are accumulated and reported at [MongoBuilder.Build] time.
// The builder is not safe for concurrent use.
type MongoBuilder struct {
	corefactory.Base
	cfg  *config.Mongodb
	errs []error

	// Dependencies
	tlsConfig         *tls.Config
	kmsProvider       kms.Provider
	healthCoordinator *health.Coordinator
	healthServiceName string
	collector         metrics.Collector
	converterOpts     []converter.Option
}

// New creates a [MongoBuilder] for the given MongoDB config.
// Config can be nil — the error surfaces at [MongoBuilder.Build] time.
func New(cfg *config.Mongodb) *MongoBuilder {
	return &MongoBuilder{
		Base: corefactory.NewBase(slog.New(slog.DiscardHandler)),
		cfg:  cfg,
	}
}

// Build creates a [mongo.Mongo] wrapper from configuration.
// If a [health.Coordinator] was provided via [MongoBuilder.UseHealthCoordinator],
// a health checker is registered under the service name "mongo".
// The returned instance is not connected; call [mongo.Mongo.Connect] to establish
// the connection.
func (b *MongoBuilder) Build(_ context.Context) (*mongo.Mongo, error) {
	if err := corefactory.JoinErrors(b.errs); err != nil {
		return nil, err
	}

	if b.cfg == nil {
		return nil, fmt.Errorf("configuration is required")
	}

	opts, err := b.createMongoOptionsFromConfig()
	if err != nil {
		return nil, err
	}

	m, err := mongo.New(b.cfg.Database, opts...)
	if err != nil {
		return nil, err
	}

	if b.healthCoordinator != nil {
		b.healthCoordinator.RegisterService(cmp.Or(b.healthServiceName, "mongo"), &mongoHealthChecker{
			m:      m,
			logger: b.Logger(),
		})
	}

	return m, nil
}

// ClientOptions creates MongoDB driver [mongoOptions.ClientOptions] from
// the builder's configuration. When [config.Mongodb.ConnectionURI] is set, ApplyURI
// is used as the base and pool/timeout/retry/TLS fields are applied on top. Otherwise,
// options are built from individual config fields including hosts, credentials, and
// compressors. Returns an error if TLS setup fails.
func (b *MongoBuilder) ClientOptions() (*mongoOptions.ClientOptions, error) {
	if b.cfg.UseConnectionURI() {
		return b.clientOptionsFromURI()
	}

	return b.clientOptionsFromFields()
}

// createMongoOptionsFromConfig creates [mongo.Option] values for the [mongo.Mongo]
// wrapper from the builder's config. The returned slice can be passed directly to [mongo.New].
func (b *MongoBuilder) createMongoOptionsFromConfig() ([]mongo.Option, error) {
	clientOpts, err := b.ClientOptions()
	if err != nil {
		return nil, err
	}

	opts := []mongo.Option{
		mongo.WithClientOptions(clientOpts),
		mongo.WithLogger(b.Logger()),
		mongo.WithCollector(b.collector),
	}
	opts = slices.AppendIfFunc(opts, len(b.converterOpts) > 0, func() []mongo.Option {
		return []mongo.Option{mongo.WithConverterOptions(b.converterOpts...)}
	})

	return b.applyEncryption(clientOpts, opts)
}

// clientOptionsFromFields builds ClientOptions from individual config fields.
func (b *MongoBuilder) clientOptionsFromFields() (*mongoOptions.ClientOptions, error) {
	clientOpts := mongoOptions.Client().
		SetHosts(b.cfg.Hosts).
		SetDirect(b.cfg.DirectConnection).
		SetMaxPoolSize(b.cfg.MaxPoolSize).
		SetMinPoolSize(b.cfg.MinPoolSize).
		SetConnectTimeout(b.cfg.ConnectTimeout).
		SetMaxConnIdleTime(b.cfg.MaxIdleTimeout).
		SetRetryReads(b.cfg.RetryReads).
		SetRetryWrites(b.cfg.RetryWrites)

	if b.cfg.ReplicaSet != "" {
		clientOpts.SetReplicaSet(b.cfg.ReplicaSet)
	}

	if len(b.cfg.Compressors) > 0 {
		clientOpts.SetCompressors(b.cfg.Compressors.StringsSlice())
		if b.cfg.ZlibCompressionLevel != -1 {
			clientOpts.SetZlibLevel(b.cfg.ZlibCompressionLevel)
		}
	}

	if b.cfg.Credentials != nil {
		cred := b.buildCredential()
		clientOpts.SetAuth(cred)
	}

	b.applyTLS(clientOpts)

	return clientOpts, nil
}

// clientOptionsFromURI builds ClientOptions starting from ApplyURI, then applies
// pool/timeout/retry/TLS/encryption fields on top.
func (b *MongoBuilder) clientOptionsFromURI() (*mongoOptions.ClientOptions, error) {
	clientOpts := mongoOptions.Client().
		ApplyURI(b.cfg.ConnectionURI.Expose()).
		SetMaxPoolSize(b.cfg.MaxPoolSize).
		SetMinPoolSize(b.cfg.MinPoolSize).
		SetConnectTimeout(b.cfg.ConnectTimeout).
		SetMaxConnIdleTime(b.cfg.MaxIdleTimeout).
		SetRetryReads(b.cfg.RetryReads).
		SetRetryWrites(b.cfg.RetryWrites)

	b.applyTLS(clientOpts)

	return clientOpts, nil
}

// applyTLS applies TLS configuration to the client options if configured.
func (b *MongoBuilder) applyTLS(clientOpts *mongoOptions.ClientOptions) {
	if b.tlsConfig != nil {
		clientOpts.SetTLSConfig(b.tlsConfig)
	}
}

// buildCredential creates MongoDB authentication credentials from the builder's configuration.
func (b *MongoBuilder) buildCredential() mongoOptions.Credential {
	creds := b.cfg.Credentials
	if creds == nil {
		return mongoOptions.Credential{}
	}

	switch creds.AuthMechanism {
	case config.MongoAuthMechanismTypeX509:
		return mongoOptions.Credential{
			AuthMechanism: config.MongoAuthMechanismTypeX509.String(),
		}
	case config.MongoAuthMechanismTypePLAIN:
		if creds.Plain == nil {
			return mongoOptions.Credential{}
		}
		return mongoOptions.Credential{
			AuthMechanism: config.MongoAuthMechanismTypePLAIN.String(),
			Username:      creds.Plain.Username,
			Password:      creds.Plain.Password.Expose(),
		}
	case config.MongoAuthMechanismTypeSCRAMSHA1, config.MongoAuthMechanismTypeSCRAMSHA256:
		if creds.Scram == nil {
			return mongoOptions.Credential{}
		}
		return mongoOptions.Credential{
			AuthMechanism: creds.AuthMechanism.String(),
			Username:      creds.Scram.Username,
			Password:      creds.Scram.Password.Expose(),
			AuthSource:    creds.Scram.AuthSource,
		}
	default:
		return mongoOptions.Credential{}
	}
}

// applyEncryption configures KMS and CSFLE encryption options when
// [config.MongodbEncryption] is present. For non-manual encryption types, it
// builds [mongoOptions.AutoEncryptionOptions] with bypass mode so reads
// transparently decrypt while writes use the explicit encryption path.
func (b *MongoBuilder) applyEncryption(clientOpts *mongoOptions.ClientOptions, opts []mongo.Option) ([]mongo.Option, error) {
	if b.cfg.Encryption == nil {
		return opts, nil
	}

	if err := b.RequireDependency(b.kmsProvider, "kms provider"); err != nil {
		return nil, err
	}

	provider := b.kmsProvider

	if b.cfg.Encryption.Type != config.MongoEncryptionTypeManual {
		vaultDB := b.cfg.Database
		if b.cfg.Encryption.VaultDatabase != nil {
			vaultDB = *b.cfg.Encryption.VaultDatabase
		}

		autoEncOpts := mongoOptions.AutoEncryption().
			SetBypassAutoEncryption(true).
			SetKeyVaultNamespace(vaultDB + "." + b.cfg.Encryption.VaultCollection).
			SetKmsProviders(provider.Credentials())

		if tlsCfg := provider.TLSConfig(); tlsCfg != nil {
			autoEncOpts.SetTLSConfig(map[string]*tls.Config{provider.Name(): tlsCfg})
		}

		clientOpts.SetAutoEncryptionOptions(autoEncOpts)
	}

	opts = append(opts,
		mongo.WithEncryptionEnabled(),
		mongo.WithKMS(provider),
		mongo.WithVaultCollection(b.cfg.Encryption.VaultCollection),
	)

	return slices.AppendIfFunc(opts, b.cfg.Encryption.VaultDatabase != nil, func() []mongo.Option {
		return []mongo.Option{mongo.WithVaultDatabase(*b.cfg.Encryption.VaultDatabase)}
	}), nil
}
