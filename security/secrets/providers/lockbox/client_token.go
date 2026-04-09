// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package lockbox

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/golang-jwt/jwt/v5"

	corectx "github.com/altessa-s/go-atlas/core/context"
	coreretry "github.com/altessa-s/go-atlas/core/runtime/retry"
)

// Yandex Cloud IAM token caching configuration
const (
	// clientTokenLifetime defines how long we cache the token before considering refresh
	// Set to 60 minutes (1 hour) as the maximum allowed lifetime
	clientTokenLifetime = 60 * time.Minute

	// earlyRefreshThreshold defines when to start attempting early token refresh
	// Refresh when 90% of lifetime has passed (54 minutes)
	earlyRefreshThreshold = 54 * time.Minute

	// refreshTimeoutSeconds defines timeout for background token refresh
	refreshTimeoutSeconds = 30

	// exponentialBackoffMultiplier for retry logic
	exponentialBackoffMultiplier = 2

	// jitterConfiguration for avoiding thundering herd
	// tokenLifetimeJitterPercent adds randomness to token lifetime (±5%)
	tokenLifetimeJitterPercent = 5

	// earlyRefreshJitterMinutes adds randomness to early refresh threshold (±15 minutes)
	earlyRefreshJitterMinutes = 15

	// jitterMath constants for calculations
	jitterMultiplier = 2   // Multiplier to create symmetric jitter range
	percentageBase   = 100 // Base for percentage calculations

	// retryConfiguration for token refresh
	tokenRefreshMaxAttempts = 3                // Maximum retry attempts for token refresh
	tokenRefreshMaxDelay    = 10 * time.Second // Maximum delay between retry attempts
)

// Token implements credential management for Yandex Cloud Lockbox API.
// It handles generating and refreshing IAM tokens for authentication with the service.
// This type implements the grpc.PerRPCCredentials interface for gRPC authentication.
type Token struct {
	// tokenMx protects access to the token and expires fields
	tokenMx sync.RWMutex

	// token is the current IAM token for authenticating with Yandex Cloud
	token string

	// expires indicates when the current token should be considered expired
	expires time.Time

	// refreshMx ensures only one goroutine attempts to refresh the token at a time
	refreshMx sync.Mutex

	// keyId is the ID of the private key used for JWT signing
	keyId string

	// serviceAccountId is the ID of the Yandex Cloud service account
	serviceAccountId string

	// privKey contains the private key data for JWT signing
	privKey []byte

	// backgroundRefreshWg tracks background refresh goroutines for proper cleanup
	backgroundRefreshWg sync.WaitGroup

	// shutdownOnce ensures shutdown is called only once
	shutdownOnce sync.Once

	// isShutdown indicates if the token manager has been shut down
	isShutdown atomic.Bool
}

// NewLockBoxToken creates a new token manager for Yandex Cloud Lockbox authentication.
//
// Parameters:
//   - keyId: ID of the private key for Yandex Cloud IAM authentication
//   - serviceKeyId: ID of the service account for Yandex Cloud IAM authentication
//   - privKey: private key content for Yandex Cloud IAM authentication
//
// Returns a configured Token instance ready for use with gRPC clients.
func NewLockBoxToken(keyId, serviceKeyId string, privKey []byte) *Token {
	return &Token{
		keyId:            keyId,
		serviceAccountId: serviceKeyId,
		privKey:          privKey,
	}
}

// GetRequestMetadata provides authentication metadata for each gRPC request.
// This method satisfies the grpc.PerRPCCredentials interface.
// It automatically refreshes the token if it has expired and attempts proactive refresh
// when the token is nearing expiration.
//
// Parameters:
//   - ctx: context for the operation
//   - uri: URIs for which the credentials will be used (not used in this implementation)
//
// Returns an authentication metadata map or an error if token refresh fails.
func (lb *Token) GetRequestMetadata(ctx context.Context, _ ...string) (map[string]string, error) {
	// Check if the token manager has been shut down
	if lb.isShutdown.Load() {
		return nil, fmt.Errorf("token manager has been shut down")
	}

	// Check if token is expired and needs immediate refresh
	if lb.isExpired() {
		if err := lb.updateClientTokenWithLock(ctx); err != nil {
			return nil, err
		}
	} else if lb.shouldRefreshEarly() && !lb.isShutdown.Load() {
		// Try proactive refresh in background, but don't block if it fails
		// Use WaitGroup to track background goroutines for proper cleanup
		lb.backgroundRefreshWg.Go(func() {
			// Create a context with timeout for the background refresh
			refreshCtx, cancel := corectx.ApplyTimeout(ctx, refreshTimeoutSeconds*time.Second)
			defer cancel()

			// Check shutdown status again to avoid unnecessary work
			if !lb.isShutdown.Load() {
				_ = lb.updateClientTokenWithLock(refreshCtx) //nolint:errcheck // Background refresh, errors are ignored
			}
		})
	}

	m := map[string]string{
		"Authorization": "Bearer " + lb.authToken(),
	}

	return m, nil
}

// RequireTransportSecurity indicates whether the credentials require transport security.
// This method satisfies the grpc.PerRPCCredentials interface.
//
// Returns true, as authentication tokens should only be sent over secure connections.
func (lb *Token) RequireTransportSecurity() bool { return true }

// Shutdown gracefully shuts down the token manager, waiting for all background
// refresh goroutines to complete. This method should be called when the token
// manager is no longer needed to prevent goroutine leaks.
//
// After calling Shutdown(), the token manager will refuse new operations
// and GetRequestMetadata will return an error.
func (lb *Token) Shutdown() {
	lb.shutdownOnce.Do(func() {
		// Mark as shut down to prevent new background goroutines
		lb.isShutdown.Store(true)

		// Wait for all background refresh goroutines to complete
		lb.backgroundRefreshWg.Wait()
	})
}

// authToken safely retrieves the current IAM token.
//
// Returns the current token protected by a read lock.
func (lb *Token) authToken() string {
	lb.tokenMx.RLock()
	defer lb.tokenMx.RUnlock()
	return lb.token
}

// isExpired checks if the current token has expired.
//
// Returns true if the token is expired or not yet initialized, false otherwise.
func (lb *Token) isExpired() bool {
	lb.tokenMx.RLock()
	defer lb.tokenMx.RUnlock()
	return lb.expires.IsZero() || lb.expires.Before(time.Now())
}

// updateClientToken refreshes the IAM token for Yandex Cloud.
// It generates a new JWT token and exchanges it for an IAM token.
//
// Parameters:
//   - ctx: context for the operation
//
// Returns an error if token generation or exchange fails.
func (lb *Token) updateClientToken(ctx context.Context) error {
	token, err := lb.generateJWTToken()
	if err != nil {
		return err
	}

	token, err = lb.exchangeJWTToken(ctx, token)
	if err != nil {
		return err
	}

	lb.tokenMx.Lock()
	defer lb.tokenMx.Unlock()
	lb.token = token
	lb.expires = time.Now().Add(calculateJitteredTokenLifetime())

	return nil
}

// generateJWTToken creates a signed JWT token for exchange with Yandex Cloud IAM.
// It uses the PS256 algorithm to sign a token containing service account information
// and expiration parameters.
//
// Returns the signed JWT token as a string or an error if signing fails.
func (lb *Token) generateJWTToken() (string, error) {
	now := time.Now().UTC()

	jwtToken := &jwt.Token{
		Header: map[string]any{"alg": "PS256", "kid": lb.keyId, "typ": "JWT"},
		Claims: jwt.MapClaims{
			"iss": lb.serviceAccountId,
			"aud": "https://iam.api.cloud.yandex.net/iam/v1/tokens",
			"iat": now.Unix(),
			"exp": now.Add(clientTokenLifetime).Unix(),
		},
		Method: jwt.SigningMethodPS256,
	}

	rsaPrivateKey, err := jwt.ParseRSAPrivateKeyFromPEM(lb.privKey)
	if err != nil {
		return "", err
	}

	tkn, err := jwtToken.SignedString(rsaPrivateKey)
	if err != nil {
		return "", err
	}

	return tkn, nil
}

// exchangeJWTToken exchanges a JWT token for an IAM token from Yandex Cloud.
// It sends an HTTP request to the IAM token service and processes the response.
//
// Parameters:
//   - ctx: context for the operation
//   - token: JWT token to exchange
//
// Returns the IAM token or an error if the exchange fails.
func (lb *Token) exchangeJWTToken(ctx context.Context, token string) (string, error) {
	client := &http.Client{
		Timeout: 10 * time.Second, // nolint:mnd
	}

	body, err := json.Marshal(struct {
		JWT string `json:"jwt"`
	}{JWT: token})
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://iam.api.cloud.yandex.net/iam/v1/tokens",
		bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)

	defer func() {
		if resp != nil {
			_, _ = io.Copy(io.Discard, resp.Body) //nolint:errcheck // intentionally draining body
			_ = resp.Body.Close()
		}
	}()

	if err != nil {
		return "", err
	}

	var data struct {
		IAMToken string `json:"iamToken"`
	}

	err = json.NewDecoder(resp.Body).Decode(&data)
	if err != nil {
		return "", err
	}

	return data.IAMToken, nil
}

// shouldRefreshEarly checks if the token should be proactively refreshed.
// Returns true when the token has passed the early refresh threshold but is not yet expired.
func (lb *Token) shouldRefreshEarly() bool {
	lb.tokenMx.RLock()
	defer lb.tokenMx.RUnlock()

	if lb.expires.IsZero() {
		return false // Token not initialized yet
	}

	now := time.Now()
	jitteredThreshold := calculateJitteredEarlyRefreshThreshold()
	earlyRefreshTime := lb.expires.Add(-jitteredThreshold)

	return now.After(earlyRefreshTime) && now.Before(lb.expires)
}

// updateClientTokenWithLock performs token refresh with proper synchronization.
// Only one goroutine can refresh the token at a time, others will wait for completion.
// Implements double-checked locking pattern to avoid unnecessary refreshes.
func (lb *Token) updateClientTokenWithLock(ctx context.Context) error {
	// Fast path: check if token is still valid (may have been refreshed by another goroutine)
	if !lb.isExpired() {
		return nil
	}

	// Acquire refresh lock to ensure only one goroutine refreshes at a time
	lb.refreshMx.Lock()
	defer lb.refreshMx.Unlock()

	// Double-check: token might have been refreshed while waiting for lock
	if !lb.isExpired() {
		return nil
	}

	// Perform token refresh with retry logic
	return lb.updateClientTokenWithRetry(ctx)
}

// updateClientTokenWithRetry refreshes the IAM token with exponential backoff retry logic.
// This helps handle transient network issues and temporary service unavailability.
func (lb *Token) updateClientTokenWithRetry(ctx context.Context) error {
	return coreretry.Do(ctx, func(ctx context.Context) error {
		return lb.updateClientToken(ctx)
	},
		coreretry.WithMaxAttempts(tokenRefreshMaxAttempts),
		coreretry.WithNextDelay(coreretry.Exponential(coreretry.ExponentialConfig{
			BaseDelay: 1 * time.Second,
			MaxDelay:  tokenRefreshMaxDelay,
			Factor:    float64(exponentialBackoffMultiplier),
		})),
	)
}

// calculateJitteredTokenLifetime calculates token lifetime with random jitter.
// This ensures different instances expire and refresh tokens at different times.
func calculateJitteredTokenLifetime() time.Duration {
	return addJitter(clientTokenLifetime, tokenLifetimeJitterPercent)
}

// addJitter adds random jitter to a duration within a percentage range.
func addJitter(d time.Duration, percent int) time.Duration {
	if percent <= 0 {
		return d
	}

	maxJitter := int64(d) * int64(percent) / percentageBase
	if maxJitter == 0 {
		return d
	}

	// Range is [-maxJitter, +maxJitter]
	jitterRange := maxJitter * jitterMultiplier
	n, err := rand.Int(rand.Reader, big.NewInt(jitterRange))
	if err != nil {
		return d
	}

	return d + time.Duration(n.Int64()-maxJitter)
}

// calculateJitteredEarlyRefreshThreshold calculates early refresh threshold with jitter.
// This spreads out when different instances start attempting early refresh.
func calculateJitteredEarlyRefreshThreshold() time.Duration {
	// Add random jitter (±15 minutes) to early refresh threshold
	maxJitter := earlyRefreshJitterMinutes * time.Minute
	jitterRange := maxJitter * jitterMultiplier
	randomJitter, err := rand.Int(rand.Reader, big.NewInt(int64(jitterRange)))
	if err != nil {
		// If random generation fails, fall back to no jitter
		return earlyRefreshThreshold
	}

	// Subtract maxJitter to make the range [-15min, +15min]
	jitter := time.Duration(randomJitter.Int64()) - maxJitter

	result := earlyRefreshThreshold + jitter

	// Ensure result is reasonable (at least 1 hour before expiration)
	minThreshold := time.Hour
	if result < minThreshold {
		return minThreshold
	}

	return result
}
