// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package auth

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/altessa-s/go-atlas/observability/metrics"

	corectx "github.com/altessa-s/go-atlas/core/context"
	coreretry "github.com/altessa-s/go-atlas/core/retry"
	vaultApi "github.com/hashicorp/vault/api"
)

// Method defines the interface that all authentication methods must implement.
// It provides a pluggable system where different authentication methods
// (AppRole, Token, UserPass, etc.) can be used interchangeably.
type Method interface {
	// Authenticate performs the authentication operation using the provided Vault client.
	// It returns a secret containing authentication details or an error if authentication fails.
	// Implementations should return errors wrapped with WrapAuthError for consistent error handling.
	Authenticate(ctx context.Context, client *vaultApi.Client) (*vaultApi.Secret, error)

	// Shutdown performs cleanup operations when the method is no longer needed.
	// This can include closing connections, clearing sensitive data, etc.
	Shutdown() error

	// Name returns a human-readable name for the authentication method.
	// This is used for logging and error reporting.
	Name() string
}

// Authenticator manages the authentication lifecycle for a Vault client.
// It handles initial authentication, token renewal, error recovery with
// exponential backoff, and graceful shutdown. The authenticator is designed
// to run continuously in a background goroutine and will automatically
// re-authenticate if the token expires or renewal fails.
//
// The authenticator uses channels for communication:
//   - FirstRenewCh: Signals when the first token has been obtained
//   - ErrorsCh: Reports authentication errors that require attention
//
// Thread Safety: Authenticator is safe for concurrent use. All channel
// operations are non-blocking to prevent deadlocks.
type Authenticator struct {
	client       *vaultApi.Client
	firstTokenCh chan struct{}
	errCh        chan error
	method       Method
	metrics      *vaultAuthMetrics
	logger       *slog.Logger
	nextBackoff  coreretry.NextDelayFunc
	authTimeout  time.Duration
	watcher      *vaultApi.LifetimeWatcher
	renewWg      sync.WaitGroup
}

// NewAuthenticator creates a new authenticator with the specified client,
// authentication method, and options. The authenticator will use the provided
// method to obtain and renew Vault tokens automatically.
// The authenticator does not start automatically; call Run() to begin.
//
// Example:
//
//	authenticator := auth.NewAuthenticator(vaultClient, authMethod,
//		auth.WithLogger(slog.Default()),
//		auth.WithBackoffBase(2*time.Second),
//	)
//	go authenticator.Run(ctx)
func NewAuthenticator(cl *vaultApi.Client, method Method, opt ...Option) *Authenticator {
	opts := newOptions(opt...)

	return &Authenticator{
		firstTokenCh: make(chan struct{}),
		errCh:        make(chan error, 1), // Buffered to prevent blocking
		client:       cl,
		method:       method,
		metrics:      newVaultAuthMetrics(opts.collector),
		logger:       opts.logger,
		nextBackoff: coreretry.Exponential(coreretry.ExponentialConfig{
			BaseDelay: opts.backoffBase,
			MaxDelay:  opts.backoffMax,
			Factor:    1, // constant base: the auth loop tracks no attempt counter
			Jitter:    1, // additive-uniform jitter in [0, base), capped at MaxDelay
		}),
		authTimeout: opts.authTimeout,
	}
}

// ErrorsCh returns a read-only channel that receives authentication errors.
// Errors sent to this channel indicate problems that require attention,
// such as invalid credentials or permanent authentication failures. It holds
// at most one unread error (later ones are dropped) and is closed when Run
// returns.
//
// Example:
//
//	select {
//	case err := <-authenticator.ErrorsCh():
//		log.Printf("auth error: %v", err)
//	case <-ctx.Done():
//	}
func (a *Authenticator) ErrorsCh() <-chan error {
	return a.errCh
}

// FirstRenewCh returns a read-only channel that is closed when the first
// token has been successfully obtained and set on the Vault client. It stays
// open if Run ends without obtaining a token; pair it with [Authenticator.ErrorsCh],
// which is closed when Run returns.
//
// Example:
//
//	<-authenticator.FirstRenewCh() // Wait for first token
//	// Now safe to use Vault client
func (a *Authenticator) FirstRenewCh() <-chan struct{} {
	return a.firstTokenCh
}

// Run starts the authentication and token renewal process.
// This method blocks and should typically be run in a goroutine.
// It handles initial authentication, token renewal, error recovery with
// backoff, and graceful shutdown when the context is canceled.
//
// Example:
//
//	ctx, cancel := context.WithCancel(context.Background())
//	defer cancel()
//	go authenticator.Run(ctx)
//	<-authenticator.FirstRenewCh()
func (a *Authenticator) Run(ctx context.Context) {
	defer func() {
		// Cleanup watcher if it exists
		a.stopWatcher()
		a.renewWg.Wait()

		// Closing errCh signals that the run has ended. firstTokenCh is left
		// open unless a token was actually obtained: closing it here would
		// report readiness after a failure, cancellation, or panic.
		close(a.errCh)
	}()

	defer func() {
		_ = a.method.Shutdown() //nolint:errcheck
	}()

	// backoffOrDone creates a timer that will fire after the given duration or
	// when the context is done, whichever happens first.
	backoffOrDone := func(ctx context.Context, backoff time.Duration) {
		timer := time.NewTimer(backoff)
		defer timer.Stop()

		select {
		case <-timer.C:
		case <-ctx.Done():
		}
	}

	// In this loop we are trying to authenticate and if it ok creating renew token loop.
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		a.logger.DebugContext(ctx, "authenticating...")

		backoff := a.nextBackoffTime()

		a.metrics.authAttempts.Inc()

		// try to authenticate with timeout
		authCtx, cancel := corectx.ApplyTimeout(ctx, a.authTimeout)
		secret, err := a.method.Authenticate(authCtx, a.client)
		cancel() // Always call cancel to release resources
		if err != nil {
			a.logger.ErrorContext(ctx, "unable to authenticate", "error", err)

			// Check if it's an authentication error that shouldn't be retried
			if IsAuthenticationError(err) {
				a.metrics.authErrors.WithLabels(metrics.Labels{"type": "permanent"}).Inc()
				a.reportError(err)
				return
			}

			// In other cases we need to wait some time before next loop iteration.
			a.metrics.authErrors.WithLabels(metrics.Labels{"type": "transient"}).Inc()
			backoffOrDone(ctx, backoff)
			continue
		}

		// If we are here, it means that we have successfully authenticated, and
		// we can create renew token loop.

		// Token not renewable, we don't need to create renew token loop.
		if !secret.Auth.Renewable || secret.Auth.LeaseDuration == 0 {
			a.logger.DebugContext(ctx, "no need to renew token, token was not renewable or lease duration was 0")

			// Signal first token received even for non-renewable tokens
			select {
			case <-a.firstTokenCh:
				// Already closed
			default:
				close(a.firstTokenCh)
			}
			return
		}

		// Stop any existing watcher
		a.stopWatcher()

		// Create new watcher.
		a.watcher, err = a.client.NewLifetimeWatcher(&vaultApi.LifetimeWatcherInput{
			Secret:        secret,
			RenewBehavior: vaultApi.RenewBehaviorIgnoreErrors,
		})

		if err != nil {
			a.logger.WarnContext(ctx, "error creating lifetime watcher, backing off and retrying",
				"error", err, "backoff", backoff)

			// We need to wait some time before next loop iteration.
			backoffOrDone(ctx, backoff)
			continue
		}

		a.logger.DebugContext(ctx, "starting auth token renewal process")

		// Run watcher loop synchronously; it returns when renewal ends or ctx cancels.
		a.runWatcher(ctx)
	}
}

// stopWatcher safely stops the current watcher if it exists
func (a *Authenticator) stopWatcher() {
	if a.watcher != nil {
		a.watcher.Stop()
		a.watcher = nil
	}
}

// runWatcher runs the watcher in a controlled goroutine with proper cleanup
func (a *Authenticator) runWatcher(ctx context.Context) {
	if a.watcher == nil {
		return
	}

	// Start the watcher renewal process
	a.renewWg.Go(func() {
		a.watcher.Renew()
	})

	for {
		select {
		case <-ctx.Done():
			a.logger.InfoContext(ctx, "context canceled, stopping watcher")
			a.stopWatcher()
			return
		case err := <-a.watcher.DoneCh():
			if err != nil {
				a.metrics.tokenRenewalErrors.Inc()
				// The raw Vault SDK error may carry lease IDs and
				// internal request URLs (role/path). Logging it
				// verbatim leaks operational topology to whoever sees
				// the warning stream. Use a stable message and log
				// the error TYPE only; the full error still
				// propagates to errCh for callers who need it.
				a.logger.WarnContext(ctx, "vault token renewal failed, backing off and retrying",
					"error_type", fmt.Sprintf("%T", err))
				a.reportError(err)
			}
			a.stopWatcher()
			return
		case <-a.watcher.RenewCh():
			a.metrics.tokenRenewals.Inc()
			a.logger.DebugContext(ctx, "auth token was successfully renewed")

			// Signal first token received
			select {
			case <-a.firstTokenCh:
				// Already closed
			default:
				close(a.firstTokenCh)
			}
		}
	}
}

// reportError delivers err on errCh without blocking. errCh holds one
// unread error; when nobody drains it (the usual case once the first token is
// obtained), later errors are dropped instead of stalling the renewal loop.
func (a *Authenticator) reportError(err error) {
	select {
	case a.errCh <- err:
	default:
	}
}

// nextBackoffTime returns the delay before the next authentication attempt:
// backoffBase plus uniform jitter in [0, backoffBase), capped at backoffMax.
func (a *Authenticator) nextBackoffTime() time.Duration {
	return a.nextBackoff(0, nil)
}
