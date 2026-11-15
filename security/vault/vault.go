// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package vault

import (
	"cmp"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/altessa-s/go-atlas-core/runtime/panics"
	"github.com/altessa-s/go-atlas/security/vault/auth"

	vaultApi "github.com/hashicorp/vault/api"
)

var (
	// ErrTimeout is returned when waiting for a Vault token times out during authentication.
	ErrTimeout = errors.New("timeout waiting for Vault token via auth method")
	// ErrRenewalStopped is returned when the renewal run ends before it obtained
	// a first token (canceled, superseded by a newer run, or the auth method
	// panicked) without reporting an authentication error.
	ErrRenewalStopped = errors.New("vault renewal stopped before a token was obtained")
	// ErrTLSConfigNotApplied is returned by New when a TLS configuration was
	// supplied but cannot be installed: the HTTP transport is not an
	// *http.Transport, or a ready-made client was supplied alongside it.
	ErrTLSConfigNotApplied = errors.New("vault TLS config cannot be applied to the client transport")
	// ErrRenewalCanceled is reported by RenewalErr when the context given to
	// RunRenewalWithContext ended after the first login, without StopRenewal
	// or Close: the token is no longer renewed and will expire.
	ErrRenewalCanceled = errors.New("vault renewal ended with its context, the token is no longer renewed")
	// ErrClosed is returned by RunRenewalWithContext after Close: the auth
	// method was shut down and cannot log in again.
	ErrClosed = errors.New("vault client is closed")
)

// DefaultRevokeTimeout bounds the token self-revocation performed by Close.
const DefaultRevokeTimeout = 5 * time.Second

// Vault is a high-level client for interacting with HashiCorp Vault.
// It manages authentication, token renewal, and provides access to the underlying Vault API client.
// Use New to create a new instance.
type Vault struct {
	opts    *options
	logger  *slog.Logger
	metrics *vaultMetrics

	// mu guards the renewal lifecycle. authDone is the completion channel of
	// the most recent renewal run; it is kept after the run ends (closed) so a
	// later run or StopRenewal can always join its predecessor.
	mu         sync.Mutex
	authCancel context.CancelFunc
	authDone   chan struct{}
	// tokenIssued records that a renewal run obtained a token from an auth
	// method that mints tokens; Close revokes only such a token.
	tokenIssued bool
	// authenticated reports that the current renewal run logged in and has
	// not ended with an error since.
	authenticated bool
	// renewalErr is the error that ended the current renewal run
	// unexpectedly; a later run that logs in clears it.
	renewalErr error
	// closed is set by Close before it shuts the auth method down; no renewal
	// run starts after it.
	closed bool
}

// errRenewalPanicked is recorded as the renewal error when the run panicked.
var errRenewalPanicked = errors.New("vault renewal run panicked")

// New creates a new Vault client with the provided options.
// Options can configure the underlying API client, TLS, authentication method, and logging.
//
// Example:
//
//	client, err := vault.New(ctx,
//		vault.WithAuthMethod(approle.New("role-id", "secret-id")),
//		vault.WithLogger(slog.Default()),
//	)
func New(ctx context.Context, opts ...Option) (*Vault, error) {
	o := newOptions(opts...)

	v := &Vault{
		opts:    o,
		logger:  cmp.Or(o.logger, slog.New(slog.DiscardHandler)),
		metrics: newVaultMetrics(o.meter),
	}

	// Initialize Vault client
	if err := v.initClient(); err != nil {
		v.logger.ErrorContext(ctx, "failed to initialize Vault client", slog.Any("error", err))
		return nil, err
	}

	if o.authMethod != nil {
		v.logger.InfoContext(ctx, "vault client initialized", "auth_method", o.authMethod.Name())
	} else {
		v.logger.InfoContext(ctx, "vault client initialized", "auth_method", "none")
	}

	if o.healthCoordinator != nil {
		o.healthCoordinator.RegisterService("vault", v)
	}

	return v, nil
}

// RunRenewalWithContext starts the authentication token renewal process in a background goroutine.
// It blocks until the first token is obtained or an error/timeout occurs.
//
// Each call starts a fresh renewal run and stops the previous one; the new run
// begins only after its predecessor has fully exited, so two runs never
// overlap. Every run re-authenticates with the configured auth method. The
// method keeps its credentials when a run ends, so renewal can be restarted
// after a failed or stopped run; [Vault.Close] shuts the method down.
//
// ctx bounds the whole run, not only the first login: when it ends later,
// without StopRenewal or Close, the token is no longer renewed and the run is
// recorded as ended with [ErrRenewalCanceled] (unless the token never
// expires, in which case the run had already ended).
//
// A run that ends on its own with an error (a login rejected for its
// credentials, for example) is logged and recorded: [Vault.RenewalErr]
// returns the error and [Vault.CheckHealth] reports not serving until a later
// call logs in again.
func (v *Vault) RunRenewalWithContext(ctx context.Context) (err error) {
	if v.opts.authMethod == nil {
		return errors.New("no authentication method configured")
	}
	if ctx == nil {
		return errors.New("renewal context cannot be nil")
	}

	done := make(chan struct{})

	// A fresh authenticator per run: its channels are closed when its run ends,
	// so they must never be shared with a later run. The method outlives the
	// run, and the login is recorded on the run's goroutine so neither the
	// auth timeout nor Close can miss a token that was issued.
	a := auth.NewAuthenticator(v.opts.vaultClient, v.opts.authMethod,
		auth.WithLogger(v.logger), auth.WithMeter(v.opts.meter),
		auth.WithKeepMethodOnExit(), auth.WithOnLogin(func() { v.recordLogin(done) }))

	// Stop the previous run (if any) and chain this run behind it.
	v.mu.Lock()
	if v.closed {
		v.mu.Unlock()
		return ErrClosed
	}
	if v.authCancel != nil {
		v.authCancel()
	}
	prevDone := v.authDone
	childCtx, cancel := context.WithCancel(ctx)
	v.authCancel = cancel
	v.authDone = done
	v.authenticated = false
	v.mu.Unlock()

	v.metrics.renewalAttempts.Inc()
	stop := v.metrics.renewalDuration.Start()

	tmout := time.NewTimer(v.opts.authTimeout)
	defer tmout.Stop()

	go func() {
		// Overwritten by the result of Run; it stays set only if Run panicked.
		runErr := errRenewalPanicked
		defer close(done)
		defer func() { v.recordRunEnd(childCtx, done, runErr) }()
		defer panics.Handle(childCtx)
		if prevDone != nil {
			<-prevDone
		}
		// Run logs the error that ended it and reports it on ErrorsCh.
		runErr = a.Run(childCtx)
	}()

	// Wait for our first token to be set on the client before returning.
	v.logger.DebugContext(ctx, "waiting for Vault token")
	select {
	case <-a.FirstRenewCh():
		stop()
		v.logger.DebugContext(ctx, "first auth token received and set")
		return nil
	case runErr, ok := <-a.ErrorsCh():
		// A token obtained before the run reported an error or ended (e.g. a
		// non-renewable token, after which the run returns) is still a success.
		select {
		case <-a.FirstRenewCh():
			stop()
			return nil
		default:
		}
		if !ok {
			runErr = ErrRenewalStopped
		}
		err = runErr
	case <-tmout.C:
		// We use a configurable timeout because the auth handler could get
		// stuck in a failure loop on the first token request if it is
		// misconfigured. This ensures that we don't block forever on
		// auth that is never going to succeed.
		err = ErrTimeout
	case <-ctx.Done():
		err = ctx.Err()
	}

	stop()
	v.metrics.renewalErrors.Inc()
	cancel()
	v.abandonRun(done)
	return err
}

// abandonRun marks the renewal run identified by done as not running after
// RunRenewalWithContext gave up on it, so a login it still completes does not
// make the client look authenticated: nothing renews that token.
func (v *Vault) abandonRun(done chan struct{}) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.authDone == done {
		v.authCancel = nil
		v.authenticated = false
		// The caller gets the reason from RunRenewalWithContext itself; a
		// cancellation the run may already have recorded would only repeat it.
		if errors.Is(v.renewalErr, ErrRenewalCanceled) {
			v.renewalErr = nil
		}
	}
}

// RawClient returns the underlying HashiCorp Vault API client for advanced operations.
//
// Example:
//
//	raw := client.RawClient()
//	secret, _ := raw.Logical().Read("secret/data/myapp")
func (v *Vault) RawClient() *vaultApi.Client {
	return v.opts.vaultClient
}

// StopRenewal cancels the background token renewal process, if running, and
// waits for it to exit. The wait is bounded as long as the auth method honors
// context cancellation.
//
// Example:
//
//	defer client.StopRenewal()
func (v *Vault) StopRenewal() error {
	v.mu.Lock()
	cancel := v.authCancel
	v.authCancel = nil
	done := v.authDone
	v.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}
	return nil
}

// Close stops token renewal, shuts the auth method down (which zeroes its
// credentials, so renewal cannot be started again) and, when the current
// token was obtained through an auth method that mints tokens (see
// [auth.TokenIssuer]), revokes it so it cannot be used after shutdown. A token supplied by the caller (the token
// auth method, or one set on a client passed to New) is never revoked.
// Revocation is best effort: it is bounded by DefaultRevokeTimeout and a
// failure is logged, not returned.
//
// Example:
//
//	defer client.Close()
func (v *Vault) Close() error {
	// Closing first keeps a concurrent RunRenewalWithContext from starting a
	// login that would overlap the method's shutdown below.
	v.mu.Lock()
	v.closed = true
	v.mu.Unlock()

	err := v.StopRenewal()

	v.mu.Lock()
	issued := v.tokenIssued
	v.tokenIssued = false
	v.mu.Unlock()

	// The renewal run has ended and no further one can log in after Close, so
	// the method's credentials are zeroed here, not when a single run ends.
	if v.opts.authMethod != nil {
		_ = v.opts.authMethod.Shutdown() //nolint:errcheck // best-effort wipe
	}

	if !issued || v.opts.vaultClient.Token() == "" {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), DefaultRevokeTimeout)
	defer cancel()

	// The argument is ignored by the API: revoke-self always acts on the client token.
	if revokeErr := v.opts.vaultClient.Auth().Token().RevokeSelfWithContext(ctx, ""); revokeErr != nil {
		// The raw SDK error may carry request URLs; log its type only.
		v.logger.WarnContext(ctx, "failed to revoke Vault token on close", "error_type", fmt.Sprintf("%T", revokeErr))
	}
	v.opts.vaultClient.ClearToken()

	return err
}

// recordLogin runs on the goroutine of the renewal run identified by done,
// right after its auth method logged in. It records that the token now on the
// client was minted by the method, whichever run obtained it, and, for the
// current run, that the client is authenticated again.
func (v *Vault) recordLogin(done chan struct{}) {
	issuer, ok := v.opts.authMethod.(auth.TokenIssuer)

	v.mu.Lock()
	defer v.mu.Unlock()
	if ok && issuer.IssuesToken() {
		v.tokenIssued = true
	}
	if v.authDone == done && v.authCancel != nil {
		v.authenticated = true
		v.renewalErr = nil
	}
}

// recordRunEnd records how the renewal run identified by done ended. A run
// that was superseded by a newer one no longer speaks for the client. A nil
// runErr is an expected end (canceled, or a token that never expires) and
// leaves the state as it is.
//
// The exception is a run that returned nil because the context given to
// RunRenewalWithContext ended while renewal was still wanted (no StopRenewal
// or Close): nothing renews the token any more, so the run is recorded as
// ended with [ErrRenewalCanceled]. A run that returned earlier because its
// token never expires is not affected by its context ending later.
func (v *Vault) recordRunEnd(ctx context.Context, done chan struct{}, runErr error) {
	v.mu.Lock()
	current := v.authDone == done
	if runErr == nil && current && v.authCancel != nil && ctx.Err() != nil {
		runErr = ErrRenewalCanceled
		v.authCancel = nil
	}
	if runErr == nil {
		v.mu.Unlock()
		return
	}
	if current {
		v.authenticated = false
		v.renewalErr = runErr
	}
	v.mu.Unlock()

	if current {
		// The raw error may carry request URLs; log its type only.
		v.logger.ErrorContext(ctx, "vault token renewal ended unexpectedly, the client is no longer authenticated",
			"error_type", fmt.Sprintf("%T", runErr))
	}
}

// RenewalErr returns the error that ended the current renewal run
// unexpectedly, or nil while the run is healthy, was stopped, or has not been
// started. A later [Vault.RunRenewalWithContext] that logs in clears it.
func (v *Vault) RenewalErr() error {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.renewalErr
}

// initClient initializes the underlying Vault API client if not already set.
func (v *Vault) initClient() error {
	// If vault client is not specified, create a new one with default config
	if v.opts.vaultClient == nil {
		vaultConfig := vaultApi.DefaultConfig()
		vaultConfig.MaxRetries = 3

		if err := applyTLSConfig(vaultConfig, v.opts.tlsConfig); err != nil {
			return err
		}

		var err error
		v.opts.vaultClient, err = vaultApi.NewClient(vaultConfig)

		return err
	}

	if v.opts.tlsConfig != nil {
		// A supplied client carries its own transport; dropping the TLS
		// config silently would leave the caller on unexpected TLS settings.
		return ErrTLSConfigNotApplied
	}

	return nil
}

// applyTLSConfig installs tlsConfig on the HTTP transport of cfg. A nil
// tlsConfig is a no-op; a transport that cannot carry it is an error.
func applyTLSConfig(cfg *vaultApi.Config, tlsConfig *tls.Config) error {
	if tlsConfig == nil {
		return nil
	}
	transport, ok := cfg.HttpClient.Transport.(*http.Transport)
	if !ok {
		return ErrTLSConfigNotApplied
	}
	transport.TLSClientConfig = tlsConfig
	return nil
}
