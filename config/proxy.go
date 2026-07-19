// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config

import (
	"fmt"
	"net/url"

	coremaps "github.com/altessa-s/go-atlas/core/collections/maps"
	grpcclient "github.com/altessa-s/go-atlas/transport/grpc/client"
	httpclient "github.com/altessa-s/go-atlas/transport/http/client"
	ozzo_rules "github.com/altessa-s/ozzo-rules"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// ProxyMode selects the proxy-resolution strategy for the
// transport/http/client and transport/grpc/client packages. The zero
// value (empty string) means "no override" — the underlying transport
// keeps its default env-based behavior. Every non-empty mode does
// something explicit.
//
// The env-based defaults differ per transport: net/http resolves via
// http.ProxyFromEnvironment (HTTP_PROXY / HTTPS_PROXY / NO_PROXY),
// while grpc-go performs its own HTTPS_PROXY / HTTP_PROXY / NO_PROXY
// lookup.
type ProxyMode string

const (
	// ProxyModeNone disables proxy resolution entirely, including the
	// standard HTTP_PROXY / HTTPS_PROXY / NO_PROXY environment lookup.
	ProxyModeNone ProxyMode = "none"

	// ProxyModeURL routes every request through a single proxy URL.
	ProxyModeURL ProxyMode = "url"

	// ProxyModeHost routes every request through a host:port pair
	// with optional credentials.
	ProxyModeHost ProxyMode = "host"
)

const (
	proxyMinPort = 1
	proxyMaxPort = 65535
)

// proxyAllowedModes enumerates every non-empty ProxyMode value
// accepted by the YAML loader. The empty zero value also passes
// validation — it means "no override" and is checked separately
// (validation.When) so OneOf does not have to know about it.
var proxyAllowedModes = []any{
	ProxyModeNone,
	ProxyModeURL,
	ProxyModeHost,
}

// proxyAllowedSchemes is the set of URL schemes accepted by the
// http/socks proxy resolver in net/http. is.URL would reject socks5,
// so Proxy carries its own schema check in Validate.
var proxyAllowedSchemes = coremaps.NewImmutableMap(map[string]struct{}{
	"http": {}, "https": {}, "socks5": {}, "socks5h": {},
})

// Proxy is the YAML-driven outbound proxy configuration shared by
// transport/http/client and transport/grpc/client. The same struct
// loads under both the http: and grpc: YAML nodes; translate it into
// client options via HTTPClientOptions or GrpcClientOptions:
//
//	opts, err := cfg.Proxy.HTTPClientOptions()
//	if err != nil {
//	    return nil, err
//	}
//	opts = append(opts, httpclient.WithLogger(logger))
//	client := httpclient.New(opts...)
//
// A nil receiver — or an empty Mode — produces no options, leaving the
// client on its env-based proxy default. Note that the two transports
// resolve that default differently: net/http honors HTTP_PROXY /
// HTTPS_PROXY / NO_PROXY via http.ProxyFromEnvironment, while grpc-go
// performs its own HTTPS_PROXY / HTTP_PROXY / NO_PROXY lookup.
type Proxy struct {
	// Mode selects which fields below are consulted; see ProxyMode
	// constants. Leave empty (the zero value) to keep the underlying
	// transport's default behavior — typically env-based proxy lookup.
	Mode ProxyMode `yaml:"mode"`

	// URL is the full proxy URL (http, https, socks5, socks5h).
	// Required when Mode is "url". Credentials may be embedded as
	// http://user:pass@host:port, but for secrets prefer Mode=host
	// with a separate Auth block — secrets in URL strings are awkward
	// to mask in logs.
	URL string `yaml:"url"`

	// Host is the proxy hostname or IP address. Required when Mode is "host".
	Host string `yaml:"host"`

	// Port is the proxy TCP port (1–65535). Required when Mode is "host".
	Port int `yaml:"port"`

	// Auth carries proxy credentials. Optional even when Mode is "host"
	// (anonymous proxies). Password uses Secret to redact in logs.
	Auth *ProxyAuth `yaml:"auth" default:"-"`
}

// ProxyAuth carries proxy credentials. Username is required when the
// block is present; Password uses Secret so it is redacted in
// fmt/JSON/YAML/slog output.
type ProxyAuth struct {
	Username string `yaml:"username"`
	Password Secret `yaml:"password"`
}

// DefaultProxy returns the zero-value Proxy. Materializing it via
// HTTPClientOptions or GrpcClientOptions produces no options, leaving
// each client on its env-based proxy default.
func DefaultProxy() Proxy {
	return Proxy{}
}

// Validate performs validation on the Proxy configuration.
//
// It enforces:
//   - Mode is one of "" (passthrough), none, url, host;
//   - the fields required by the selected Mode are present (URL for url,
//     Host+Port for host);
//   - the fields irrelevant to the selected Mode are absent (mutual
//     exclusion); this catches YAML typos that would otherwise be
//     silently dropped.
//
// A nil receiver is valid and reports no error.
func (p *Proxy) Validate() error {
	if p == nil {
		return nil
	}

	if err := p.validateExclusion(); err != nil {
		return err
	}

	return ValidateStruct(p,
		validation.Field(&p.Mode,
			validation.When(p.Mode != "", ozzo_rules.OneOf(proxyAllowedModes...))),
		validation.Field(&p.URL,
			validation.When(p.Mode == ProxyModeURL,
				validation.Required, validation.By(validateProxyURL))),
		validation.Field(&p.Host,
			validation.When(p.Mode == ProxyModeHost, validation.Required)),
		validation.Field(&p.Port,
			validation.When(p.Mode == ProxyModeHost,
				validation.Required, validation.Min(proxyMinPort), validation.Max(proxyMaxPort))),
		validation.Field(&p.Auth),
	)
}

// validateProxyURL is a validation.RuleFunc that accepts a syntactically
// valid URL with a scheme net/http knows how to use as a proxy
// (http, https, socks5, socks5h). is.URL rejects socks5 so we roll our
// own.
func validateProxyURL(value any) error {
	raw, _ := value.(string)
	if raw == "" {
		return nil
	}
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("must be a valid URL: %w", err)
	}
	if u.Scheme == "" || u.Host == "" {
		return fmt.Errorf("must be an absolute URL with scheme and host")
	}
	if !proxyAllowedSchemes.Contains(u.Scheme) {
		return fmt.Errorf("unsupported proxy scheme %q (allowed: http, https, socks5, socks5h)", u.Scheme)
	}
	if u.Port() == "" {
		// Stdlib's http.Transport defaults missing ports via portMap
		// (80/443/1080), but our custom DialContext does not — passing
		// a port-less Host through to net.Dial returns "missing port
		// in address" at every dial. Surface the misconfiguration at
		// load time instead.
		return fmt.Errorf("must include an explicit port (e.g. %s:3128)", u.Host)
	}
	return nil
}

// validateExclusion rejects fields that are set but ignored by the
// current Mode — those are almost always typos in the YAML.
func (p *Proxy) validateExclusion() error {
	if p.Mode != ProxyModeURL && p.URL != "" {
		return fmt.Errorf("Proxy: url must be empty when mode is %q", p.Mode)
	}
	if p.Mode != ProxyModeHost {
		if p.Host != "" {
			return fmt.Errorf("Proxy: host must be empty when mode is %q", p.Mode)
		}
		if p.Port != 0 {
			return fmt.Errorf("Proxy: port must be empty when mode is %q", p.Mode)
		}
		if p.Auth != nil {
			return fmt.Errorf("Proxy: auth must be empty when mode is %q", p.Mode)
		}
	}
	return nil
}

// HTTPClientOptions materializes the proxy configuration into a slice
// of transport/http/client options ready to be passed to httpclient.New.
//
// A nil receiver or empty Mode returns (nil, nil) so the client keeps
// the http.ProxyFromEnvironment default (HTTP_PROXY / HTTPS_PROXY /
// NO_PROXY).
//
// An invalid URL in Mode url surfaces as an error here rather than at
// request time.
func (p *Proxy) HTTPClientOptions() ([]httpclient.Option, error) {
	return proxyClientOptions(p,
		httpclient.WithoutProxy, httpclient.WithProxyURL, httpclient.WithProxy)
}

// GrpcClientOptions materializes the proxy configuration into a slice
// of transport/grpc/client options ready to be passed to grpcclient.New.
//
// A nil receiver or empty Mode returns (nil, nil) so the client keeps
// grpc-go's own HTTPS_PROXY / HTTP_PROXY / NO_PROXY env default.
//
// An invalid URL in Mode url surfaces as an error here rather than at
// dial time.
func (p *Proxy) GrpcClientOptions() ([]grpcclient.Option, error) {
	return proxyClientOptions(p,
		grpcclient.WithoutProxy, grpcclient.WithProxyURL, grpcclient.WithProxy)
}

// proxyClientOptions folds the Mode switch shared by HTTPClientOptions
// and GrpcClientOptions over the transport-specific option
// constructors.
func proxyClientOptions[O any](
	p *Proxy,
	withoutProxy func() O,
	withProxyURL func(*url.URL) O,
	withProxy func(string, int, *url.Userinfo) O,
) ([]O, error) {
	if p == nil {
		return nil, nil
	}
	switch p.Mode {
	case "":
		return nil, nil
	case ProxyModeNone:
		return []O{withoutProxy()}, nil
	case ProxyModeURL:
		u, err := url.Parse(p.URL)
		if err != nil {
			return nil, fmt.Errorf("Proxy: parse url: %w", err)
		}
		return []O{withProxyURL(u)}, nil
	case ProxyModeHost:
		return []O{withProxy(p.Host, p.Port, p.userinfo())}, nil
	default:
		return nil, fmt.Errorf("Proxy: unknown mode %q", p.Mode)
	}
}

// userinfo builds a *url.Userinfo from the Auth block, returning nil
// for anonymous proxies. Password is exposed (it has to be — the
// underlying URL needs the plain text) only here.
func (p *Proxy) userinfo() *url.Userinfo {
	if p.Auth == nil || p.Auth.Username == "" {
		return nil
	}
	if p.Auth.Password.IsEmpty() {
		return url.User(p.Auth.Username)
	}
	return url.UserPassword(p.Auth.Username, p.Auth.Password.Expose())
}

// Validate enforces that the credentials block, when present, names a
// user. Password may be empty (some proxy auth schemes accept
// username-only). A nil receiver is valid.
func (a *ProxyAuth) Validate() error {
	if a == nil {
		return nil
	}
	return ValidateStruct(a,
		validation.Field(&a.Username, validation.Required),
	)
}
