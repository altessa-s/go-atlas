// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oauth2client

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
)

// URL schemes of the endpoint check.
const (
	schemeHTTP  = "http"
	schemeHTTPS = "https"
)

// DefaultHTTPTimeout bounds every request of the built-in HTTP client, used
// when no client is injected.
const DefaultHTTPTimeout = 30 * time.Second

// defaultHTTPClient issues the requests when no client is injected. Unlike
// [http.DefaultClient] it has a timeout and never follows a redirect: a 307 or
// 308 would re-POST the form, client secret included, to wherever the
// response points.
var defaultHTTPClient = &http.Client{
	Timeout:       DefaultHTTPTimeout,
	CheckRedirect: noRedirect,
}

// defaultSecureHTTPClient is defaultHTTPClient behind the https-only guard.
var defaultSecureHTTPClient = httpsOnly(defaultHTTPClient)

// endpointClient returns the HTTP client every flow issues its requests with: the
// injected one, or the built-in client. Unless the grant permits plain HTTP it is
// wrapped so a request to a plain-http endpoint fails with
// [ErrInsecureEndpoint] instead of sending credentials in the clear.
//
// An injected client is copied with its redirect policy tightened by
// [guardRedirects], whether or not the grant permits plain HTTP.
func endpointClient(client *http.Client, insecureHTTP bool) *http.Client {
	switch {
	case insecureHTTP && client != nil:
		return guardRedirects(client)
	case insecureHTTP:
		return defaultHTTPClient
	case client != nil:
		return httpsOnly(guardRedirects(client))
	default:
		return defaultSecureHTTPClient
	}
}

// noRedirect is the redirect policy of the built-in client: the response of
// the redirecting endpoint is returned as is.
func noRedirect(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }

// guardRedirects returns a copy of client that cannot carry a request to
// another origin. Every flow POSTs credentials (client secret, refresh token,
// subject token, client assertion), and a 307 or 308 re-sends that body to
// wherever the response points.
//
// A client without a CheckRedirect gets the policy of the built-in client and
// follows no redirect at all. A client with its own CheckRedirect keeps it,
// but a hop to an origin (scheme, host and port) other than that of the
// original request fails with [ErrRedirectNotAllowed] before the policy is
// consulted.
func guardRedirects(client *http.Client) *http.Client {
	clone := *client
	prior := client.CheckRedirect
	if prior == nil {
		clone.CheckRedirect = noRedirect
		return &clone
	}
	clone.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) == 0 || !sameOrigin(via[0].URL, req.URL) {
			return coreerrs.Wrapf(ErrRedirectNotAllowed, "redirect to %q leaves the origin of the request", req.URL.Host)
		}
		return prior(req, via)
	}
	return &clone
}

// sameOrigin reports whether to has the scheme, host and port of from and
// carries no user info.
func sameOrigin(from, to *url.URL) bool {
	return from != nil && to != nil && to.User == nil &&
		from.Scheme == to.Scheme &&
		strings.EqualFold(from.Hostname(), to.Hostname()) &&
		originPort(from) == originPort(to)
}

// originPort returns the port of u, defaulted from its scheme.
func originPort(u *url.URL) string {
	if port := u.Port(); port != "" {
		return port
	}
	switch u.Scheme {
	case schemeHTTPS:
		return "443"
	case schemeHTTP:
		return "80"
	default:
		return ""
	}
}

// httpsOnly returns a copy of client whose transport rejects insecure URLs.
func httpsOnly(client *http.Client) *http.Client {
	clone := *client
	clone.Transport = &httpsOnlyTransport{base: client.Transport}
	return &clone
}

// httpsOnlyTransport refuses a request whose URL fails [checkEndpointURL]. It
// sits on the transport so it covers every endpoint of every flow, the
// x/oauth2-backed ones included, and each redirect hop of an injected client.
type httpsOnlyTransport struct {
	base http.RoundTripper
}

// RoundTrip forwards req to the wrapped transport when its URL is secure.
func (t *httpsOnlyTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if err := checkEndpointURL(req.URL); err != nil {
		if req.Body != nil {
			_ = req.Body.Close()
		}
		return nil, err
	}
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(req)
}

// ValidateEndpointURL reports whether rawURL may carry client credentials:
// it must be an absolute https URL. Plain http is accepted for a loopback
// host ("localhost" or a loopback IP literal), where the request never leaves
// the machine, and for any host when allowInsecureHTTP is set. A rejected URL
// returns an error wrapping [ErrInsecureEndpoint].
func ValidateEndpointURL(rawURL string, allowInsecureHTTP bool) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return coreerrs.Wrap(ErrInsecureEndpoint, "endpoint is not a valid URL")
	}
	if allowInsecureHTTP && u.Scheme == schemeHTTP && u.Host != "" {
		return nil
	}
	return checkEndpointURL(u)
}

// validateEndpoint is [ValidateEndpointURL] for a constructor: name says
// which of its endpoints was rejected.
func validateEndpoint(name, rawURL string, insecureHTTP bool) error {
	if err := ValidateEndpointURL(rawURL, insecureHTTP); err != nil {
		return coreerrs.Wrap(err, name)
	}
	return nil
}

// checkEndpointURL accepts an https URL and an http URL on a loopback host.
func checkEndpointURL(u *url.URL) error {
	switch {
	case u.Host == "":
		return &insecureEndpointError{message: "endpoint must be an absolute https URL"}
	case u.Scheme == schemeHTTPS, u.Scheme == schemeHTTP && isLoopbackHost(u.Hostname()):
		return nil
	default:
		return &insecureEndpointError{message: fmt.Sprintf("endpoint host %q must use https", u.Host)}
	}
}

type insecureEndpointError struct {
	message string
}

func (e *insecureEndpointError) Error() string { return e.message + ": " + ErrInsecureEndpoint.Error() }
func (e *insecureEndpointError) Unwrap() error { return ErrInsecureEndpoint }

// permanentRequestError reports whether a request error cannot be cured by
// sending the request again: the endpoint is insecure, or it redirects to
// another origin.
func permanentRequestError(err error) bool {
	return errors.Is(err, ErrInsecureEndpoint) || errors.Is(err, ErrRedirectNotAllowed)
}

// isLoopbackHost reports whether host (without port) is "localhost" or a
// loopback IP literal, the rule auth/oidc applies to loopback issuers.
func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// logError renders err for a log line. The text of an [*oauth2.RetrieveError]
// embeds the whole IdP response body, which may echo request parameters; it is
// replaced as [redactError] describes.
func logError(err error) string {
	return redactError(err).Error()
}
