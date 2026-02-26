// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"context"
	"crypto/tls"
	"fmt"

	"github.com/altessa-s/go-atlas/config"
	"github.com/altessa-s/go-atlas/core/collections/slices"
	"github.com/altessa-s/go-atlas/data/mongo"

	corefactory "github.com/altessa-s/go-atlas/core/factory"
	kmsfactory "github.com/altessa-s/go-atlas/data/mongo/kms/factory"
	tlsfactory "github.com/altessa-s/go-atlas/security/tlsutils/factory"
	mongoOptions "go.mongodb.org/mongo-driver/v2/mongo/options"
)

// Factory creates MongoDB clients and driver options from [config.Mongodb].
// It delegates TLS configuration to [tlsfactory.Factory] and KMS/CSFLE
// configuration to [kmsfactory.Factory], both of which are optional.
type Factory struct {
	corefactory.Base
	opts       *options
	tlsFactory *tlsfactory.Factory
	kmsFactory *kmsfactory.Factory
}

// New creates a new Factory with the given functional options.
// By default the factory uses a discard logger and no TLS or KMS factories;
// provide [WithLogger], [WithTlsFactory], and [WithKmsFactory] to override.
func New(opts ...Option) *Factory {
	cfg := newOptions(opts...)
	return &Factory{
		Base:       corefactory.NewBase(cfg.logger),
		opts:       cfg,
		tlsFactory: cfg.tlsFactory,
		kmsFactory: cfg.kmsFactory,
	}
}

// ClientOptionsFromConfig creates MongoDB driver [mongoOptions.ClientOptions] from
// configuration. When [config.Mongodb.ConnectionURI] is set, ApplyURI is used as the
// base and pool/timeout/retry/TLS fields are applied on top. Otherwise, options are
// built from individual config fields including hosts, credentials, and compressors.
// Returns an error if cfg is nil or TLS setup fails.
func (f *Factory) ClientOptionsFromConfig(cfg *config.Mongodb) (*mongoOptions.ClientOptions, error) {
	if cfg == nil {
		return nil, fmt.Errorf("configuration is required")
	}

	if cfg.UseConnectionURI() {
		return f.clientOptionsFromURI(cfg)
	}

	return f.clientOptionsFromFields(cfg)
}

// clientOptionsFromFields builds ClientOptions from individual config fields.
func (f *Factory) clientOptionsFromFields(cfg *config.Mongodb) (*mongoOptions.ClientOptions, error) {
	clientOpts := mongoOptions.Client().
		SetHosts(cfg.Hosts).
		SetDirect(cfg.DirectConnection).
		SetMaxPoolSize(cfg.MaxPoolSize).
		SetMinPoolSize(cfg.MinPoolSize).
		SetConnectTimeout(cfg.ConnectTimeout).
		SetMaxConnIdleTime(cfg.MaxIdleTimeout).
		SetRetryReads(cfg.RetryReads).
		SetRetryWrites(cfg.RetryWrites)

	if cfg.ReplicaSet != "" {
		clientOpts.SetReplicaSet(cfg.ReplicaSet)
	}

	if len(cfg.Compressors) > 0 {
		clientOpts.SetCompressors(cfg.Compressors.StringsSlice())
		if cfg.ZlibCompressionLevel != -1 {
			clientOpts.SetZlibLevel(cfg.ZlibCompressionLevel)
		}
	}

	if cfg.Credentials != nil {
		cred := f.buildCredential(cfg.Credentials)
		clientOpts.SetAuth(cred)
	}

	if err := f.applyTLS(clientOpts, cfg); err != nil {
		return nil, err
	}

	return clientOpts, nil
}

// clientOptionsFromURI builds ClientOptions starting from ApplyURI, then applies
// pool/timeout/retry/TLS/encryption fields on top.
func (f *Factory) clientOptionsFromURI(cfg *config.Mongodb) (*mongoOptions.ClientOptions, error) {
	clientOpts := mongoOptions.Client().
		ApplyURI(cfg.ConnectionURI.Expose()).
		SetMaxPoolSize(cfg.MaxPoolSize).
		SetMinPoolSize(cfg.MinPoolSize).
		SetConnectTimeout(cfg.ConnectTimeout).
		SetMaxConnIdleTime(cfg.MaxIdleTimeout).
		SetRetryReads(cfg.RetryReads).
		SetRetryWrites(cfg.RetryWrites)

	if err := f.applyTLS(clientOpts, cfg); err != nil {
		return nil, err
	}

	return clientOpts, nil
}

// applyTLS applies TLS configuration to the client options if configured.
func (f *Factory) applyTLS(clientOpts *mongoOptions.ClientOptions, cfg *config.Mongodb) error {
	if cfg.TLS != nil {
		if f.tlsFactory == nil {
			return f.WrapError(nil, "tls config provided but tls factory is not configured")
		}
		tlsConfig, err := f.tlsFactory.CreateClientConfigFromConfig(cfg.TLS)
		if err != nil {
			return f.WrapError(err, "failed to create TLS config")
		}
		if tlsConfig != nil {
			clientOpts.SetTLSConfig(tlsConfig)
		}
	}

	return nil
}

// buildCredential creates MongoDB authentication credentials from configuration.
func (f *Factory) buildCredential(creds *config.MongodbCredentials) mongoOptions.Credential {
	if creds == nil {
		return mongoOptions.Credential{}
	}

	switch creds.AuthMechanism {
	case config.MongoAuthMechanismTypeX509:
		return mongoOptions.Credential{
			AuthMechanism: "MONGODB-X509",
		}
	case config.MongoAuthMechanismTypePLAIN:
		if creds.Plain == nil {
			return mongoOptions.Credential{}
		}
		return mongoOptions.Credential{
			AuthMechanism: "PLAIN",
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

// CreateMongoOptionsFromConfig creates [mongo.Option] values for the [mongo.Mongo]
// wrapper from configuration. The returned slice can be passed directly to [mongo.New].
// If [config.Mongodb.Encryption] is configured, KMS and vault options are included.
// For [config.MongoEncryptionTypeAuto], the driver client is configured to transparently
// decrypt encrypted fields on reads while writes use the explicit encryption path.
func (f *Factory) CreateMongoOptionsFromConfig(cfg *config.Mongodb) ([]mongo.Option, error) {
	clientOpts, err := f.ClientOptionsFromConfig(cfg)
	if err != nil {
		return nil, err
	}

	opts := []mongo.Option{
		mongo.WithClientOptions(clientOpts),
		mongo.WithLogger(f.Logger()),
	}

	return f.applyEncryption(cfg, clientOpts, opts)
}

// applyEncryption configures KMS and CSFLE encryption options when
// [config.MongodbEncryption] is present. For non-manual encryption types, it
// builds [mongoOptions.AutoEncryptionOptions] with bypass mode so reads
// transparently decrypt while writes use the explicit encryption path.
func (f *Factory) applyEncryption(cfg *config.Mongodb, clientOpts *mongoOptions.ClientOptions, opts []mongo.Option) ([]mongo.Option, error) {
	if cfg.Encryption == nil {
		return opts, nil
	}

	kf := f.kmsFactory
	if kf == nil {
		kf = kmsfactory.New()
	}

	provider, err := kf.CreateProviderFromConfig(cfg.Encryption.KMS)
	if err != nil {
		return nil, f.WrapError(err, "failed to create KMS provider")
	}

	if cfg.Encryption.Type != config.MongoEncryptionTypeManual {
		vaultDB := cfg.Database
		if cfg.Encryption.VaultDatabase != nil {
			vaultDB = *cfg.Encryption.VaultDatabase
		}

		autoEncOpts := mongoOptions.AutoEncryption().
			SetBypassAutoEncryption(true).
			SetKeyVaultNamespace(vaultDB + "." + cfg.Encryption.VaultCollection).
			SetKmsProviders(provider.Credentials())

		if tlsCfg := provider.TLSConfig(); tlsCfg != nil {
			autoEncOpts.SetTLSConfig(map[string]*tls.Config{provider.Name(): tlsCfg})
		}

		clientOpts.SetAutoEncryptionOptions(autoEncOpts)
	}

	opts = append(opts,
		mongo.WithEncryptionEnabled(),
		mongo.WithKMS(provider),
		mongo.WithVaultCollection(cfg.Encryption.VaultCollection),
	)

	return slices.AppendIfFunc(opts, cfg.Encryption.VaultDatabase != nil, func() []mongo.Option {
		return []mongo.Option{mongo.WithVaultDatabase(*cfg.Encryption.VaultDatabase)}
	}), nil
}

// CreateMongoFromConfig creates a [mongo.Mongo] wrapper from configuration.
// It delegates to [Factory.CreateMongoFromConfigWithContext] with [context.Background].
// The returned instance is not connected; call [mongo.Mongo.Connect] to establish
// the connection.
func (f *Factory) CreateMongoFromConfig(cfg *config.Mongodb) (*mongo.Mongo, error) {
	return f.CreateMongoFromConfigWithContext(context.Background(), cfg)
}

// CreateMongoFromConfigWithContext creates a [mongo.Mongo] wrapper from configuration.
// If a [health.Coordinator] was provided via [WithHealthCoordinator], a health checker
// is registered under the service name "mongo".
// The returned instance is not connected; call [mongo.Mongo.Connect] to establish
// the connection.
func (f *Factory) CreateMongoFromConfigWithContext(_ context.Context, cfg *config.Mongodb) (*mongo.Mongo, error) {
	opts, err := f.CreateMongoOptionsFromConfig(cfg)
	if err != nil {
		return nil, err
	}

	m, err := mongo.New(cfg.Database, opts...)
	if err != nil {
		return nil, err
	}

	if f.opts.healthCoordinator != nil {
		f.opts.healthCoordinator.RegisterService("mongo", &mongoHealthChecker{m: m})
	}

	return m, nil
}
