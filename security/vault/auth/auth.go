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

	corectx "github.com/altessa-s/go-atlas-core/context"
	coreretry "github.com/altessa-s/go-atlas-core/retry"
	vaultApi "github.com/hashicorp/vault/api"
)

// Method defines the interface that all authentication methods must implement.
// It provides a pluggable system where different authentication methods
// (AppRole, Token, UserPass, etc.) can be used interchangeably.
type Method interface {
	// Authenticate performs the authentication operation using the provided Vault client.
	// It returns a secret containing authentication details or an error if authentication fails.
	// Implementations should return errors wrapped with WrapAuthError for consistent error handling.
	// On success the secret must carry Auth: a nil secret or nil Auth makes
	// [Authenticator.Run] report [ErrEmptyResponse] and stop.
	Authenticate(ctx context.Context, client *vaultApi.Client) (*vaultApi.Secret, error)

	// Shutdown performs cleanup operations when the method is no longer needed.
	// This can include closing connections, clearing sensitive data, etc.
	Shutdown() error

	// Name returns a human-readable name for the authentication method.
	// This is used for logging and error reporting.
	Name() string
}

// Schedule for refreshing a non-renewable token after a successful login. It
// is independent of the failure backoff, which applies only after a failed
// login or a failed watcher setup.
const (
	// reauthTTLFraction is the share of a non-renewable token's remaining
	// lease after which [Authenticator.Run] logs in again.
	reauthTTLFraction = 2.0 / 3.0
	// reauthSafetyMargin is the time before expiry by which the next login
	// must have started.
	reauthSafetyMargin = 250 * time.Millisecond
	// reauthMinInterval is the shortest wait between two successful logins.
	// It only keeps the loop from spinning on a lease too short to refresh
	// ahead of expiry.
	reauthMinInterval = 100 * time.Millisecond
)

// backoffFactor multiplies the failure backoff after every consecutive failed
// login.
const backoffFactor = 2

// healthyCycleFraction is the share of the lease granted at login that a
// renewable token must have been held for, renewals included, before the
// cycle counts as healthy and resets the failure backoff.
const healthyCycleFraction = 0.5

// reauthDelay returns how long to wait before logging in again when a
// non-renewable token has remaining left on its lease. tooShort reports a
// lease that cannot be refreshed ahead of expiry; the delay is then
// reauthMinInterval.
func reauthDelay(remaining time.Duration) (delay time.Duration, tooShort bool) {
	latest := remaining - reauthSafetyMargin
	if latest < reauthMinInterval {
		return reauthMinInterval, true
	}
	return max(min(time.Duration(float64(remaining)*reauthTTLFraction), latest), reauthMinInterval), false
}

// TokenIssuer is an optional interface for a [Method] whose Authenticate logs
// in and mints a new token, as opposed to one that installs a token supplied
// by the caller. The Vault client revokes only tokens from an issuing method
// when it is closed.
type TokenIssuer interface {
	// IssuesToken reports whether Authenticate mints a token owned by this client.
	IssuesToken() bool
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
	// shortTTLWarned limits the unusably-short-TTL warning to one per
	// authenticator; only Run reads and writes it.
	shortTTLWarned   bool
	keepMethodOnExit bool
	onLogin          func()
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
	if opts.onLogin == nil {
		opts.onLogin = func() {}
	}

	return &Authenticator{
		firstTokenCh: make(chan struct{}),
		errCh:        make(chan error, 1), // Buffered to prevent blocking
		client:       cl,
		method:       method,
		metrics:      newVaultAuthMetrics(opts.meter),
		logger:       opts.logger,
		// The delay doubles with every consecutive failure Run counts, up to
		// backoffMax, so replicas do not hammer a recovering Vault with
		// logins. Jitter adds up to the delay itself, capped at the maximum.
		nextBackoff: coreretry.Exponential(
			opts.backoffBase,
			coreretry.WithExponentialMaxDelay(opts.backoffMax),
			coreretry.WithExponentialFactor(backoffFactor),
			coreretry.WithExponentialJitter(1),
		),
		authTimeout:      opts.authTimeout,
		keepMethodOnExit: opts.keepMethodOnExit,
		onLogin:          opts.onLogin,
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
// A non-renewable token with a finite TTL keeps the run alive when the method
// mints its tokens (it is a [TokenIssuer]): Run logs in
// again after two thirds of the remaining lease, and no later than a short
// safety margin before expiry, so the client never holds an expired token.
// The failure backoff does not delay this refresh. A lease too short to
// refresh ahead of expiry is logged once as a warning and refreshed every
// 100ms.
//
// A failed login that may succeed later is retried after a backoff that
// starts at the configured base ([WithBackoffBase]), doubles with every
// consecutive failure up to the maximum ([WithBackoffMax]) and carries
// jitter. A login rejected for its credentials ([IsAuthenticationError]) ends
// the run. The backoff is reset by a login whose token was put to use: a
// non-renewable token at once, a renewable one when its renewal cycle held it
// for at least half of the lease granted at login. A renewable token whose
// cycle ends earlier (renewal failing right after login) counts as one more
// failure, so such logins are spaced by the same growing backoff, never
// waiting past the lease of the token still held.
//
// A method that installs a token supplied by the caller cannot replace it:
// for such a non-renewable token with a finite TTL Run logs the coming expiry
// once as a warning, does not log in again, and returns an error wrapping
// [ErrTokenExpired] when the TTL has passed.
//
// When Run returns it shuts the auth method down, which zeroes its
// credentials, unless [WithKeepMethodOnExit] is set. Run returns nil when the context is canceled or the token never
// expires; any other exit returns the error that ended the run and logs it.
//
// Example:
//
//	ctx, cancel := context.WithCancel(context.Background())
//	defer cancel()
//	go authenticator.Run(ctx)
//	<-authenticator.FirstRenewCh()
func (a *Authenticator) Run(ctx context.Context) (err error) {
	defer func() {
		if err != nil {
			// The error itself was logged where it occurred; this line marks
			// that no further tokens will be obtained.
			a.logger.ErrorContext(ctx, "vault authenticator stopped unexpectedly, tokens are no longer refreshed",
				"method", a.method.Name(), "error_type", fmt.Sprintf("%T", err))
		}
	}()

	defer func() {
		// Cleanup watcher if it exists
		a.stopWatcher()
		a.renewWg.Wait()

		// Closing errCh signals that the run has ended. firstTokenCh is left
		// open unless a token was actually obtained: closing it here would
		// report readiness after a failure, cancellation, or panic.
		close(a.errCh)
	}()

	defer a.shutdownMethod()

	// failures counts consecutive failed attempts to obtain a usable token;
	// it drives the exponential failure backoff and is reset once a login
	// took effect.
	failures := 0

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
			return nil
		default:
		}

		a.logger.DebugContext(ctx, "authenticating...")

		backoff := a.nextBackoff(failures, nil)
		failures++

		a.metrics.authAttempts.Inc()

		// try to authenticate with timeout
		authStarted := time.Now()
		authCtx, cancel := corectx.ApplyTimeout(ctx, a.authTimeout)
		secret, err := a.method.Authenticate(authCtx, a.client)
		cancel() // Always call cancel to release resources
		if err == nil && (secret == nil || secret.Auth == nil) {
			// A method that succeeds without auth info breaks the Method
			// contract; retrying cannot fix it, so it is reported as permanent.
			err = WrapAuthError(a.method.Name(), ErrEmptyResponse)
			a.logger.ErrorContext(ctx, "auth method returned no auth info", "method", a.method.Name())
			a.metrics.authErrors.WithLabels(metrics.Labels{labelType: "permanent"}).Inc()
			a.reportError(err)
			return err
		}
		if err != nil {
			a.logger.ErrorContext(ctx, "unable to authenticate", "error", err)

			// Check if it's an authentication error that shouldn't be retried
			if IsAuthenticationError(err) {
				a.metrics.authErrors.WithLabels(metrics.Labels{labelType: "permanent"}).Inc()
				a.reportError(err)
				return err
			}

			// In other cases we need to wait some time before next loop iteration.
			a.metrics.authErrors.WithLabels(metrics.Labels{labelType: "transient"}).Inc()
			backoffOrDone(ctx, backoff)
			continue
		}

		// If we are here, it means that we have successfully authenticated, and
		// we can create renew token loop.
		a.onLogin()

		// Token not renewable, we don't need to create renew token loop.
		if !secret.Auth.Renewable || secret.Auth.LeaseDuration <= 0 {
			failures = 0

			// Signal first token received even for non-renewable tokens
			select {
			case <-a.firstTokenCh:
				// Already closed
			default:
				close(a.firstTokenCh)
			}

			if secret.Auth.LeaseDuration <= 0 {
				a.logger.DebugContext(ctx, "no need to renew token, lease duration was 0")
				return nil
			}

			ttl := time.Duration(secret.Auth.LeaseDuration) * time.Second
			if !issuesToken(a.method) {
				// Logging in again would hand back the same token with less
				// time left: there is nothing to refresh.
				return a.holdUntilExpiry(ctx, ttl-time.Since(authStarted))
			}

			// A non-renewable token with a finite TTL expires on its own: log
			// in again at two thirds of the remaining lease, the point at
			// which the lifetime watcher renews renewable tokens. The lease
			// is counted from the start of the login, so the time the login
			// took is not spent twice.
			reauthIn := a.reauthIn(ctx, ttl, authStarted)
			a.logger.DebugContext(ctx, "token is not renewable, re-authenticating before it expires", "reauth_in", reauthIn)
			backoffOrDone(ctx, reauthIn)
			continue
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

		// A cycle that gave up its token early is a failure like a failed
		// login: only a cycle that held the token resets the backoff.
		failures = a.settleCycle(ctx, time.Duration(secret.Auth.LeaseDuration)*time.Second, authStarted, backoff, failures)
	}
}

// settleCycle decides what follows a renewal cycle that ended (the lifetime
// watcher stopped, with or without an error) and returns the failure count
// for the next login: zero after a healthy cycle, failures unchanged
// otherwise. A cycle is healthy when it held its token for at least
// healthyCycleFraction of the lease the login granted, renewals included; it
// is followed by an immediate login.
//
// A shorter cycle (renewal failing right after login, or a lease that cannot
// be extended) would otherwise make Run log in again at once, in a tight loop
// against Vault. It counts as a failure: settleCycle waits out the failure
// backoff, which grows with consecutive failures, before the next login. The
// wait never runs past the lease of the token still held (less the safety
// margin), since logging in again is the only way left to keep a valid token;
// reauthMinInterval keeps the loop bounded when that lease is nearly spent.
func (a *Authenticator) settleCycle(
	ctx context.Context, ttl time.Duration, authStarted time.Time, backoff time.Duration, failures int,
) int {
	held := time.Since(authStarted)
	if ctx.Err() != nil || float64(held) >= float64(ttl)*healthyCycleFraction {
		return 0
	}

	wait := max(min(backoff, ttl-held-reauthSafetyMargin), reauthMinInterval)
	a.logger.WarnContext(ctx, "vault token renewal ended early, backing off before logging in again",
		"method", a.method.Name(), "held", held, "ttl", ttl, "backoff", wait)

	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-ctx.Done():
	}
	return failures
}

// reauthIn returns how long Run waits before logging in again for a
// non-renewable token of the given ttl obtained by a login that began at
// authStarted. A lease too short to refresh ahead of expiry is reported once.
func (a *Authenticator) reauthIn(ctx context.Context, ttl time.Duration, authStarted time.Time) time.Duration {
	delay, tooShort := reauthDelay(ttl - time.Since(authStarted))
	if tooShort && !a.shortTTLWarned {
		a.shortTTLWarned = true
		a.logger.WarnContext(ctx, "vault token TTL is too short to refresh before it expires, the token may lapse between logins",
			"method", a.method.Name(), "ttl", ttl, "reauth_in", delay)
	}
	return delay
}

// issuesToken reports whether a login of m mints a new token (see
// [TokenIssuer]), as opposed to installing the one the caller supplied.
func issuesToken(m Method) bool {
	issuer, ok := m.(TokenIssuer)
	return ok && issuer.IssuesToken()
}

// holdUntilExpiry ends the run for a non-renewable token that the method
// cannot replace: it reports the coming expiry once and waits for it. It
// returns nil when ctx ends first and an error wrapping [ErrTokenExpired]
// once the token has expired.
func (a *Authenticator) holdUntilExpiry(ctx context.Context, remaining time.Duration) error {
	a.logger.WarnContext(ctx, "vault token is not renewable and the auth method cannot obtain a new one, it will expire",
		"method", a.method.Name(), "expires_in", remaining)

	timer := time.NewTimer(remaining)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return nil
	case <-timer.C:
	}

	err := NewAuthError(a.method.Name(), "token expired", 0, ErrTokenExpired)
	a.logger.ErrorContext(ctx, "vault token expired and the auth method cannot obtain a new one", "method", a.method.Name())
	a.reportError(err)
	return err
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

// shutdownMethod retires the auth method when Run returns, unless the method
// outlives the run ([WithKeepMethodOnExit]).
func (a *Authenticator) shutdownMethod() {
	if a.keepMethodOnExit {
		return
	}
	_ = a.method.Shutdown() //nolint:errcheck
}
