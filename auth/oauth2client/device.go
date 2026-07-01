// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oauth2client

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"golang.org/x/oauth2"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// DeviceFlow drives the device authorization grant (RFC 8628) for
// input-constrained clients — a CLI, a TV, or any device without a browser.
// The three steps are: [DeviceFlow.Authorize] requests a code, the user is shown
// the returned verification URL and user code to enter on a second device, and
// [DeviceFlow.Token] polls the token endpoint until the user approves.
//
// Client authentication uses the [WithAuthStyle] secret (device clients are
// commonly public, so an empty secret is normal); [WithClientAuth] JWT
// assertions do not apply here. A DeviceFlow is immutable and safe for
// concurrent use.
type DeviceFlow struct {
	cfg     oauth2.Config
	client  *http.Client
	metrics *Metrics
	logger  *slog.Logger
}

// NewDeviceFlow builds a [DeviceFlow] for the IdP endpoint and registered client.
// endpoint must carry the device authorization URL ([oauth2.Endpoint.DeviceAuthURL])
// and the token URL; clientSecret may be "" for a public client. Requested
// scopes and a custom [http.Client] come from the [Option] values.
func NewDeviceFlow(endpoint oauth2.Endpoint, clientID, clientSecret string, opts ...Option) *DeviceFlow {
	o := newOptions(opts...)
	return &DeviceFlow{
		cfg: oauth2.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			Endpoint:     endpoint,
			Scopes:       o.scopes,
		},
		client:  o.httpClient,
		metrics: o.metrics,
		logger:  o.logger,
	}
}

// Authorize performs the device authorization request and returns the codes and
// verification URL to present to the user. Show VerificationURI (or, when set,
// VerificationURIComplete, which embeds the user code) together with UserCode,
// then pass the response to [DeviceFlow.Token].
func (d *DeviceFlow) Authorize(ctx context.Context) (*oauth2.DeviceAuthResponse, error) {
	resp, err := d.cfg.DeviceAuth(withHTTPClient(ctx, d.client))
	if err != nil {
		return nil, coreerrs.Wrap(errors.Join(ErrDeviceAuth, err), "device authorization request")
	}
	return resp, nil
}

// Token polls the token endpoint until the user approves the request, honoring
// the RFC 8628 poll interval and slow_down responses. It returns a
// self-refreshing [oauth2.TokenSource] (backed by the granted refresh token) and
// the raw [oauth2.Token]. A denial, expiry, or transport error returns an error
// wrapping [ErrDeviceAuth]; the underlying *oauth2.RetrieveError with the RFC
// 8628 error code stays reachable via errors.As.
func (d *DeviceFlow) Token(ctx context.Context, da *oauth2.DeviceAuthResponse) (oauth2.TokenSource, *oauth2.Token, error) {
	ctx = withHTTPClient(ctx, d.client)
	start := time.Now()
	tok, err := d.cfg.DeviceAccessToken(ctx, da)
	d.metrics.recordFetch(grantDeviceCode, err == nil, time.Since(start))
	if err != nil {
		if d.logger != nil {
			d.logger.Warn("oauth2client: device token poll failed", "err", err)
		}
		return nil, nil, coreerrs.Wrap(errors.Join(ErrDeviceAuth, err), "device access token")
	}
	return measure(d.cfg.TokenSource(ctx, tok), grantDeviceCode, d.metrics, d.logger), tok, nil
}
