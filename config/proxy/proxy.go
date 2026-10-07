// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package proxyconfig

import (
	"fmt"
	"net/url"

	"github.com/altessa-s/go-atlas/core/types/redacted"

	validationconfig "github.com/altessa-s/go-atlas/config/validation"
	coremaps "github.com/altessa-s/go-atlas/core/collections/maps"
	ozzo_rules "github.com/altessa-s/ozzo-rules"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// Mode selects the proxy-resolution strategy for the
// transport/http/client and transport/grpc/client packages. The zero
// value (empty string) means "no override" — the underlying transport
// keeps its default env-based behavior. Every non-empty mode does
// something explicit.
//
// The env-based defaults differ per transport: net/http resolves via
// http.ProxyFromEnvironment (HTTP_PROXY / HTTPS_PROXY / NO_PROXY),
// while grpc-go performs its own HTTPS_PROXY / HTTP_PROXY / NO_PROXY
// lookup.
type Mode string

const (
	// ModeNone disables proxy resolution entirely, including the
	// standard HTTP_PROXY / HTTPS_PROXY / NO_PROXY environment lookup.
	ModeNone Mode = "none"

	// ModeURL routes every request through a single proxy URL.
	ModeURL Mode = "url"

	// ModeHost routes every request through a host:port pair
	// with optional credentials.
	ModeHost Mode = "host"
)

const (
	proxyMinPort = 1
	proxyMaxPort = 65535
)

// proxyAllowedModes enumerates every non-empty Mode value
// accepted by the YAML loader. The empty zero value also passes
// validation — it means "no override" and is checked separately
// (validation.When) so OneOf does not have to know about it.
var proxyAllowedModes = []any{
	ModeNone,
	ModeURL,
	ModeHost,
}

// proxyAllowedSchemes is the set of URL schemes accepted by the
// http/socks proxy resolver in net/http. is.URL would reject socks5,
// so Proxy carries its own schema check in Validate.
var proxyAllowedSchemes = coremaps.NewImmutableMap(map[string]struct{}{
	"http": {}, "https": {}, "socks5": {}, "socks5h": {},
})

// Config is the shared outbound proxy configuration. It contains schema and
// validation only; transport/proxydial/factory converts it into client options.
// Empty Mode retains the client's environment-based proxy defaults.
type Config struct {
	// Mode selects which fields below are consulted; see Mode
	// constants. Leave empty (the zero value) to keep the underlying
	// transport's default behavior — typically env-based proxy lookup.
	Mode Mode `yaml:"mode"`

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
	Auth *Auth `yaml:"auth" default:"-"`
}

// Auth carries proxy credentials. Username is required when the
// block is present; Password uses Secret so it is redacted in
// fmt/JSON/YAML/slog output.
type Auth struct {
	Username string                  `yaml:"username"`
	Password redacted.RedactedString `yaml:"password"`
}

// Default returns the zero-value, environment-based proxy configuration.
func Default() Config {
	return Config{}
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
func (p *Config) Validate() error {
	if p == nil {
		return nil
	}

	if err := p.validateExclusion(); err != nil {
		return err
	}

	return validationconfig.ValidateStruct(p,
		validation.Field(&p.Mode,
			validation.When(p.Mode != "", ozzo_rules.OneOf(proxyAllowedModes...))),
		validation.Field(&p.URL,
			validation.When(p.Mode == ModeURL,
				validation.Required, validation.By(validateProxyURL))),
		validation.Field(&p.Host,
			validation.When(p.Mode == ModeHost, validation.Required)),
		validation.Field(&p.Port,
			validation.When(p.Mode == ModeHost,
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
func (p *Config) validateExclusion() error {
	if p.Mode != ModeURL && p.URL != "" {
		return fmt.Errorf("proxy: url must be empty when mode is %q", p.Mode)
	}
	if p.Mode != ModeHost {
		if p.Host != "" {
			return fmt.Errorf("proxy: host must be empty when mode is %q", p.Mode)
		}
		if p.Port != 0 {
			return fmt.Errorf("proxy: port must be empty when mode is %q", p.Mode)
		}
		if p.Auth != nil {
			return fmt.Errorf("proxy: auth must be empty when mode is %q", p.Mode)
		}
	}
	return nil
}

// Validate enforces that the credentials block, when present, names a
// user. Password may be empty (some proxy auth schemes accept
// username-only). A nil receiver is valid.
func (a *Auth) Validate() error {
	if a == nil {
		return nil
	}
	return validationconfig.ValidateStruct(a,
		validation.Field(&a.Username, validation.Required),
	)
}
