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

	coretime "github.com/altessa-s/go-atlas/core/time"
	vaultApi "github.com/hashicorp/vault/api"
)

// ErrTimeout is returned when waiting for a Vault token times out during authentication.
var (
	ErrTimeout = errors.New("timeout waiting for Vault token via auth method")
)

// Vault is a high-level client for interacting with HashiCorp Vault.
// It manages authentication, token renewal, and provides access to the underlying Vault API client.
// Use New to create a new instance.
type Vault struct {
	opts       *options
	auth       *auth.Authenticator
	authCancel context.CancelFunc
	logger     *slog.Logger
	metrics    *vaultMetrics
	mu         sync.Mutex
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

	// If we have an auth method, create the authenticator for it.
	// Authenticator checks auth method and periodically update auth token if needed.
	if o.authMethod != nil {
		v.auth = auth.NewAuthenticator(o.vaultClient, o.authMethod, auth.WithLogger(v.logger), auth.WithCollector(o.collector))
		v.logger.InfoContext(ctx, "vault client initialized", "auth_method", o.authMethod.Name())
	} else {
		v.logger.InfoContext(ctx, "vault client initialized", "auth_method", "none")
	}

	if o.healthCoordinator != nil {
		o.healthCoordinator.RegisterService("vault", v)
	}

	return v, nil
}

// RunRenewal starts the authentication token renewal process in a background goroutine.
// It blocks until the first token is obtained or an error/timeout occurs.
// Returns an error if authentication fails or times out.
//
// Example:
//
//	if err := client.RunRenewal(); err != nil {
//		log.Fatal(err)
//	}
//	defer client.StopRenewal()
func (v *Vault) RunRenewal() (err error) {
	//nolint:contextcheck // Convenience wrapper; use RunRenewalWithContext when you have an inherited ctx.
	return v.RunRenewalWithContext(context.Background())
}

// RunRenewalWithContext starts the authentication token renewal process in a background goroutine.
// It blocks until the first token is obtained or an error/timeout occurs.
func (v *Vault) RunRenewalWithContext(ctx context.Context) (err error) {
	if v.auth == nil {
		return errors.New("no authentication method configured")
	}
	if ctx == nil {
		return errors.New("renewal context cannot be nil")
	}

	// Ensure only one renewal loop is running at a time.
	// If RunRenewalWithContext is called again, stop the previous run.
	v.mu.Lock()
	if v.authCancel != nil {
		v.authCancel()
		v.authCancel = nil
	}

	// Create the context we'll use to cancel this run.
	childCtx, cancel := context.WithCancel(ctx)
	v.authCancel = cancel
	v.mu.Unlock()

	v.metrics.renewalAttempts.Inc()
	stop := v.metrics.renewalDuration.Start()

	tmout := time.NewTimer(v.opts.authTimeout)
	defer coretime.TimerStopAndDrain(tmout)

	// Start the auth handler
	go func() {
		defer panics.Handle(childCtx)
		v.auth.Run(childCtx)
	}()

	// Wait for our first token to be set on the client before returning
	v.logger.DebugContext(ctx, "waiting for Vault token")
	select {
	case <-v.auth.FirstRenewCh():
		stop()
		v.logger.DebugContext(ctx, "first auth token received and set")
	case err = <-v.auth.ErrorsCh():
		stop()
		v.metrics.renewalErrors.Inc()
		cancel()
		return
	case <-tmout.C:
		// We use a configurable timeout because the auth handler could get
		// stuck in a failure loop on the first token request if it is
		// misconfigured. This ensures that we don't block forever on
		// auth that is never going to succeed.
		stop()
		v.metrics.renewalErrors.Inc()
		cancel()
		err = ErrTimeout
		return
	case <-ctx.Done():
		stop()
		v.metrics.renewalErrors.Inc()
		cancel()
		err = ctx.Err()
		return
	}

	return
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

// StopRenewal cancels the background token renewal process, if running.
//
// Example:
//
//	defer client.StopRenewal()
func (v *Vault) StopRenewal() error {
	v.mu.Lock()
	cancel := v.authCancel
	v.authCancel = nil
	v.mu.Unlock()

	if cancel != nil {
		cancel()
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
