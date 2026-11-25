// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config_test

import (
	"errors"
	"maps"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"

	authconfig "github.com/altessa-s/go-atlas/config/auth"
	brokerconfig "github.com/altessa-s/go-atlas/config/broker"
	clickhouseconfig "github.com/altessa-s/go-atlas/config/clickhouse"
	clienthealthconfig "github.com/altessa-s/go-atlas/config/clienthealth"
	grpcconfig "github.com/altessa-s/go-atlas/config/grpc"
	grpcclientconfig "github.com/altessa-s/go-atlas/config/grpcclient"
	httpconfig "github.com/altessa-s/go-atlas/config/http"
	idempotencyconfig "github.com/altessa-s/go-atlas/config/idempotency"
	limiterconfig "github.com/altessa-s/go-atlas/config/limiter"
	lockconfig "github.com/altessa-s/go-atlas/config/lock"
	meilisearchconfig "github.com/altessa-s/go-atlas/config/meilisearch"
	mongoconfig "github.com/altessa-s/go-atlas/config/mongo"
	natsconfig "github.com/altessa-s/go-atlas/config/nats"
	observabilityconfig "github.com/altessa-s/go-atlas/config/observability"
	pluginsconfig "github.com/altessa-s/go-atlas/config/plugins"
	probfilterconfig "github.com/altessa-s/go-atlas/config/probfilter"
	proxyconfig "github.com/altessa-s/go-atlas/config/proxy"
	redisconfig "github.com/altessa-s/go-atlas/config/redis"
	retryconfig "github.com/altessa-s/go-atlas/config/retry"
	s3config "github.com/altessa-s/go-atlas/config/s3"
	sagaconfig "github.com/altessa-s/go-atlas/config/saga"
	schedulerconfig "github.com/altessa-s/go-atlas/config/scheduler"
	secretsconfig "github.com/altessa-s/go-atlas/config/secrets"
	tlsconfig "github.com/altessa-s/go-atlas/config/tls"
	vaultconfig "github.com/altessa-s/go-atlas/config/vault"
	webhookconfig "github.com/altessa-s/go-atlas/config/webhook"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// validatable is a schema whose Validate has a pointer receiver (a value
// receiver is in the pointer method set too).
type validatable[T any] interface {
	*T
	Validate() error
}

// validateDefault validates a copy of a Default* constructor's result.
func validateDefault[T any, P validatable[T]](v T) error {
	return P(&v).Validate()
}

// TestDefaults_PassOwnValidate pins that every Default* constructor in
// config/ returns a schema its own Validate accepts: a default the schema
// rejects is unusable as a starting point and hides which field the caller
// must still set.
func TestDefaults_PassOwnValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		validate func() error
	}{
		{"authconfig.DefaultDenylist", func() error { return validateDefault(authconfig.DefaultDenylist()) }},
		{"authconfig.DefaultMTLS", func() error { return validateDefault(authconfig.DefaultMTLS()) }},
		{"authconfig.DefaultOIDCCache", func() error { return validateDefault(authconfig.DefaultOIDCCache()) }},
		{"authconfig.DefaultOIDCJwks", func() error { return validateDefault(authconfig.DefaultOIDCJwks()) }},
		{"authconfig.DefaultOIDCRevocation", func() error { return validateDefault(authconfig.DefaultOIDCRevocation()) }},
		{"authconfig.DefaultScopeRegistry", func() error { return validateDefault(authconfig.DefaultScopeRegistry()) }},
		{"authconfig.DefaultOAuth2ClientRetry", func() error { return validateDefault(authconfig.DefaultOAuth2ClientRetry()) }},
		{"authconfig.DefaultOPACache", func() error { return validateDefault(authconfig.DefaultOPACache()) }},
		{"brokerconfig.Default", func() error { return validateDefault(brokerconfig.Default()) }},
		{"clickhouseconfig.Default", func() error { return validateDefault(clickhouseconfig.Default()) }},
		{"grpcconfig.Default", func() error { return validateDefault(grpcconfig.Default()) }},
		{"grpcconfig.DefaultAuthInterceptor", func() error { return validateDefault(grpcconfig.DefaultAuthInterceptor()) }},
		{"grpcconfig.DefaultCacheInterceptor", func() error { return validateDefault(grpcconfig.DefaultCacheInterceptor()) }},
		{"grpcconfig.DefaultErrStatusInterceptor", func() error { return validateDefault(grpcconfig.DefaultErrStatusInterceptor()) }},
		{"grpcconfig.DefaultFieldMaskInterceptor", func() error { return validateDefault(grpcconfig.DefaultFieldMaskInterceptor()) }},
		{"grpcconfig.DefaultGeoACLInterceptor", func() error { return validateDefault(grpcconfig.DefaultGeoACLInterceptor()) }},
		{"grpcconfig.DefaultHealthInterceptor", func() error { return validateDefault(grpcconfig.DefaultHealthInterceptor()) }},
		{"grpcconfig.DefaultIdempotencyInterceptor", func() error { return validateDefault(grpcconfig.DefaultIdempotencyInterceptor()) }},
		{"grpcconfig.DefaultIPACLInterceptor", func() error { return validateDefault(grpcconfig.DefaultIPACLInterceptor()) }},
		{"grpcconfig.DefaultLimiterInterceptor", func() error { return validateDefault(grpcconfig.DefaultLimiterInterceptor()) }},
		{"grpcconfig.DefaultLoggerInterceptor", func() error { return validateDefault(grpcconfig.DefaultLoggerInterceptor()) }},
		{"grpcconfig.DefaultMetricsInterceptor", func() error { return validateDefault(grpcconfig.DefaultMetricsInterceptor()) }},
		{"grpcconfig.DefaultRealIPInterceptor", func() error { return validateDefault(grpcconfig.DefaultRealIPInterceptor()) }},
		{"grpcconfig.DefaultRecoveryInterceptor", func() error { return validateDefault(grpcconfig.DefaultRecoveryInterceptor()) }},
		{"grpcconfig.DefaultRequestIDInterceptor", func() error { return validateDefault(grpcconfig.DefaultRequestIDInterceptor()) }},
		{"grpcconfig.DefaultTracingInterceptor", func() error { return validateDefault(grpcconfig.DefaultTracingInterceptor()) }},
		{"grpcconfig.DefaultInterceptors", func() error { return validateDefault(grpcconfig.DefaultInterceptors()) }},
		{"grpcclientconfig.Default", func() error { return validateDefault(grpcclientconfig.Default()) }},
		{"grpcclientconfig.DefaultResolver", func() error { return validateDefault(grpcclientconfig.DefaultResolver()) }},
		{"grpcclientconfig.DefaultConnectBackoff", func() error { return validateDefault(grpcclientconfig.DefaultConnectBackoff()) }},
		{"httpconfig.Default", func() error { return validateDefault(httpconfig.Default()) }},
		{"httpconfig.DefaultClientSSRF", func() error { return validateDefault(httpconfig.DefaultClientSSRF()) }},
		{"httpconfig.DefaultBodyLimitMiddleware", func() error { return validateDefault(httpconfig.DefaultBodyLimitMiddleware()) }},
		{"httpconfig.DefaultCORSMiddleware", func() error { return validateDefault(httpconfig.DefaultCORSMiddleware()) }},
		{"httpconfig.DefaultGeoACLMiddleware", func() error { return validateDefault(httpconfig.DefaultGeoACLMiddleware()) }},
		{"httpconfig.DefaultIdempotencyMiddleware", func() error { return validateDefault(httpconfig.DefaultIdempotencyMiddleware()) }},
		{"httpconfig.DefaultIPACLMiddleware", func() error { return validateDefault(httpconfig.DefaultIPACLMiddleware()) }},
		{"httpconfig.DefaultLimiterMiddleware", func() error { return validateDefault(httpconfig.DefaultLimiterMiddleware()) }},
		{"httpconfig.DefaultLoggerMiddleware", func() error { return validateDefault(httpconfig.DefaultLoggerMiddleware()) }},
		{"httpconfig.DefaultMetricsMiddleware", func() error { return validateDefault(httpconfig.DefaultMetricsMiddleware()) }},
		{"httpconfig.DefaultRealIPMiddleware", func() error { return validateDefault(httpconfig.DefaultRealIPMiddleware()) }},
		{"httpconfig.DefaultRecoveryMiddleware", func() error { return validateDefault(httpconfig.DefaultRecoveryMiddleware()) }},
		{"httpconfig.DefaultRequestIDMiddleware", func() error { return validateDefault(httpconfig.DefaultRequestIDMiddleware()) }},
		{"httpconfig.DefaultSecurityHeadersMiddleware", func() error { return validateDefault(httpconfig.DefaultSecurityHeadersMiddleware()) }},
		{"httpconfig.DefaultTracingMiddleware", func() error { return validateDefault(httpconfig.DefaultTracingMiddleware()) }},
		{"httpconfig.DefaultMiddlewares", func() error { return validateDefault(httpconfig.DefaultMiddlewares()) }},
		{"httpconfig.DefaultPprof", func() error { return validateDefault(httpconfig.DefaultPprof()) }},
		{"limiterconfig.DefaultTokenBucketDefaultRule", func() error { return validateDefault(limiterconfig.DefaultTokenBucketDefaultRule()) }},
		{"limiterconfig.DefaultRequestRate", func() error { return validateDefault(limiterconfig.DefaultRequestRate()) }},
		{"lockconfig.DefaultDistributionLockMongo", func() error { return validateDefault(lockconfig.DefaultDistributionLockMongo()) }},
		{"lockconfig.DefaultDistributionLockNATS", func() error { return validateDefault(lockconfig.DefaultDistributionLockNATS()) }},
		{"lockconfig.DefaultLeaderElector", func() error { return validateDefault(lockconfig.DefaultLeaderElector()) }},
		{"meilisearchconfig.Default", func() error { return validateDefault(meilisearchconfig.Default()) }},
		{"natsconfig.DefaultRecovery", func() error { return validateDefault(natsconfig.DefaultRecovery()) }},
		{"natsconfig.Default", func() error { return validateDefault(natsconfig.Default()) }},
		{"observabilityconfig.DefaultHealth", func() error { return validateDefault(observabilityconfig.DefaultHealth()) }},
		{"observabilityconfig.DefaultLoggerBuffer", func() error { return validateDefault(observabilityconfig.DefaultLoggerBuffer()) }},
		{"observabilityconfig.DefaultLogger", func() error { return validateDefault(observabilityconfig.DefaultLogger()) }},
		{"observabilityconfig.DefaultMetrics", func() error { return validateDefault(observabilityconfig.DefaultMetrics()) }},
		{"observabilityconfig.Default", func() error { return validateDefault(observabilityconfig.Default()) }},
		{"observabilityconfig.DefaultTracing", func() error { return validateDefault(observabilityconfig.DefaultTracing()) }},
		{"observabilityconfig.DefaultTracingAdapters", func() error { return validateDefault(observabilityconfig.DefaultTracingAdapters()) }},
		{"observabilityconfig.DefaultTracingSampler", func() error { return validateDefault(observabilityconfig.DefaultTracingSampler()) }},
		{"observabilityconfig.DefaultTracingOTLP", func() error { return validateDefault(observabilityconfig.DefaultTracingOTLP()) }},
		{"observabilityconfig.DefaultTracingConsole", func() error { return validateDefault(observabilityconfig.DefaultTracingConsole()) }},
		{"pluginsconfig.Default", func() error { return validateDefault(pluginsconfig.Default()) }},
		{"probfilterconfig.NewBloomDefaults", func() error { return validateDefault(probfilterconfig.NewBloomDefaults()) }},
		{"probfilterconfig.NewCuckooDefaults", func() error { return validateDefault(probfilterconfig.NewCuckooDefaults()) }},
		{"probfilterconfig.NewDefaults", func() error { return validateDefault(probfilterconfig.NewDefaults()) }},
		{"probfilterconfig.Default", func() error { return validateDefault(probfilterconfig.Default()) }},
		{"proxyconfig.Default", func() error { return validateDefault(proxyconfig.Default()) }},
		{"redisconfig.Default", func() error { return validateDefault(redisconfig.Default()) }},
		{"retryconfig.Default", func() error { return validateDefault(retryconfig.Default()) }},
		{"sagaconfig.DefaultStorage", func() error { return sagaconfig.DefaultStorage().Validate() }},
		{"sagaconfig.Default", func() error { return validateDefault(sagaconfig.Default()) }},
		{"schedulerconfig.DefaultConcurrency", func() error { return validateDefault(schedulerconfig.DefaultConcurrency()) }},
		{"schedulerconfig.DefaultTaskConcurrency", func() error { return validateDefault(schedulerconfig.DefaultTaskConcurrency()) }},
		{"schedulerconfig.DefaultStorageSQLConfig", func() error { return validateDefault(schedulerconfig.DefaultStorageSQLConfig()) }},
		{"schedulerconfig.DefaultStorageConfig", func() error { return validateDefault(schedulerconfig.DefaultStorageConfig()) }},
		{"schedulerconfig.Default", func() error { return validateDefault(schedulerconfig.Default()) }},
		{"secretsconfig.Default", func() error { return validateDefault(secretsconfig.Default()) }},
		{"secretsconfig.DefaultVault", func() error { return validateDefault(secretsconfig.DefaultVault()) }},
		{"tlsconfig.DefaultProviderOCSP", func() error { return validateDefault(tlsconfig.DefaultProviderOCSP()) }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.NoError(t, tc.validate())
		})
	}
}

// TestDefaults_OnlyCallerInputMissing pins the Default* constructors that
// cannot pass Validate on their own: each leaves blank only the fields the
// caller must supply (an endpoint, a credential, a name, a provider choice
// with no default), and fails on nothing else.
func TestDefaults_OnlyCallerInputMissing(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		validate func() error
		missing  []string
	}{
		{"authconfig.Default", func() error { return validateDefault(authconfig.Default()) }, []string{"OIDC", "OPA"}},
		{"authconfig.DefaultOAuth2Client", func() error { return validateDefault(authconfig.DefaultOAuth2Client()) }, []string{"ClientId", "DiscoveryUrl", "TokenUrl"}},
		{"authconfig.DefaultOIDC", func() error { return validateDefault(authconfig.DefaultOIDC()) }, []string{"DiscoveryUrl"}},
		{"authconfig.DefaultOPA", func() error { return validateDefault(authconfig.DefaultOPA()) }, []string{"BundlePath"}},
		{"authconfig.DefaultOPAGitLab", func() error { return validateDefault(authconfig.DefaultOPAGitLab()) }, []string{"Endpoint", "ProjectID", "Token"}},
		{"authconfig.DefaultSPIFFE", func() error { return validateDefault(authconfig.DefaultSPIFFE()) }, []string{"AllowedTrustDomains"}},
		{"clienthealthconfig.Default", func() error { return validateDefault(clienthealthconfig.Default()) }, []string{"ServiceName"}},
		{"clienthealthconfig.DefaultGRPC", func() error { return validateDefault(clienthealthconfig.DefaultGRPC()) }, []string{"ServiceName"}},
		{"clienthealthconfig.DefaultHTTP", func() error { return validateDefault(clienthealthconfig.DefaultHTTP()) }, []string{"ServiceName"}},
		{"idempotencyconfig.Default", func() error { return validateDefault(idempotencyconfig.Default()) }, []string{"Storage"}},
		{"limiterconfig.DefaultBudget", func() error { return validateDefault(limiterconfig.DefaultBudget()) }, []string{"Limit", "Storage"}},
		{"limiterconfig.DefaultTokenBucket", func() error { return validateDefault(limiterconfig.DefaultTokenBucket()) }, []string{"Rules", "Storage"}},
		{"lockconfig.DefaultDistributionLock", func() error { return validateDefault(lockconfig.DefaultDistributionLock()) }, []string{"Provider"}},
		{"mongoconfig.Default", func() error { return validateDefault(mongoconfig.Default()) }, []string{"Database"}},
		{"s3config.Default", func() error { return validateDefault(s3config.Default()) }, []string{"AccessKey", "Region", "SecretKey"}},
		{"vaultconfig.Default", func() error { return validateDefault(vaultconfig.Default()) }, []string{"Auth"}},
		{"vaultconfig.DefaultAuth", func() error { return validateDefault(vaultconfig.DefaultAuth()) }, []string{"Token"}},
		{"webhookconfig.DefaultSignature", func() error { return validateDefault(webhookconfig.DefaultSignature()) }, []string{"Scheme", "Secret"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			verrs, ok := errors.AsType[validation.Errors](tc.validate())
			require.True(t, ok, "want validation.Errors")
			require.ElementsMatch(t, tc.missing, slices.Collect(maps.Keys(verrs)))
		})
	}
}
