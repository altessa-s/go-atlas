// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package vault

import (
	"cmp"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/altessa-s/go-atlas/core/runtime/panics"
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
)

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
}

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
		metrics: newVaultMetrics(o.collector),
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
// overlap. Every run re-authenticates with the configured auth method, and the
// authenticator shuts the method down when a run ends — methods that zero
// their credentials on Shutdown (token, approle, userpass) therefore cannot be
// restarted after a previous run ended.
func (v *Vault) RunRenewalWithContext(ctx context.Context) (err error) {
	if v.opts.authMethod == nil {
		return errors.New("no authentication method configured")
	}
	if ctx == nil {
		return errors.New("renewal context cannot be nil")
	}

	// A fresh authenticator per run: its channels are closed when its run ends,
	// so they must never be shared with a later run.
	a := auth.NewAuthenticator(v.opts.vaultClient, v.opts.authMethod,
		auth.WithLogger(v.logger), auth.WithCollector(v.opts.collector))

	// Stop the previous run (if any) and chain this run behind it.
	v.mu.Lock()
	if v.authCancel != nil {
		v.authCancel()
	}
	prevDone := v.authDone
	childCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	v.authCancel = cancel
	v.authDone = done
	v.mu.Unlock()

	v.metrics.renewalAttempts.Inc()
	stop := v.metrics.renewalDuration.Start()

	tmout := time.NewTimer(v.opts.authTimeout)
	defer tmout.Stop()

	go func() {
		defer close(done)
		defer panics.Handle(childCtx)
		if prevDone != nil {
			<-prevDone
		}
		a.Run(childCtx)
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
	return err
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

// initClient initializes the underlying Vault API client if not already set.
func (v *Vault) initClient() error {
	// If vault client is not specified, create a new one with default config
	if v.opts.vaultClient == nil {
		vaultConfig := vaultApi.DefaultConfig()
		vaultConfig.MaxRetries = 3

		if v.opts.tlsConfig != nil {
			if transport, ok := vaultConfig.HttpClient.Transport.(*http.Transport); ok {
				transport.TLSClientConfig = v.opts.tlsConfig
			}
		}

		var err error
		v.opts.vaultClient, err = vaultApi.NewClient(vaultConfig)

		return err
	}

	return nil
}
