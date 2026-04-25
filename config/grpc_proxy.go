// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config

import (
	"fmt"
	"net/url"

	grpcclient "github.com/altessa-s/go-atlas/transport/grpc/client"
	ozzo_rules "github.com/altessa-s/ozzo-rules"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// GrpcProxyMode selects the proxy-resolution strategy for the
// transport/grpc/client package. The zero value (empty string) means
// "no override" — grpc-go keeps its default HTTPS_PROXY/HTTP_PROXY/
// NO_PROXY env-based behavior. Every non-empty mode does something
// explicit.
type GrpcProxyMode string

const (
	// GrpcProxyModeNone disables proxy resolution entirely, including
	// the standard HTTPS_PROXY / HTTP_PROXY / NO_PROXY environment lookup.
	GrpcProxyModeNone GrpcProxyMode = "none"

	// GrpcProxyModeURL routes the connection through a single proxy URL.
	GrpcProxyModeURL GrpcProxyMode = "url"

	// GrpcProxyModeHost routes the connection through a host:port pair
	// with optional credentials.
	GrpcProxyModeHost GrpcProxyMode = "host"
)

// grpcProxyAllowedModes enumerates every non-empty GrpcProxyMode value
// accepted by the YAML loader. The empty zero value also passes
// validation — it means "no override" and is checked separately
// (validation.When) so OneOf does not have to know about it.
var grpcProxyAllowedModes = []any{
	GrpcProxyModeNone,
	GrpcProxyModeURL,
	GrpcProxyModeHost,
}

// GrpcProxy is the YAML-driven outbound proxy configuration for
// transport/grpc/client. Translate it into client options via
// ClientOptions and pass them to grpcclient.New:
//
//	opts, err := cfg.GrpcProxy.ClientOptions()
//	if err != nil {
//	    return nil, err
//	}
//	opts = append(opts, grpcclient.WithLogger(logger))
//	c, err := grpcclient.New(ctx, addr, opts...)
//
// A nil receiver — or an empty Mode — produces no options, leaving the
// client on its env-based proxy default.
type GrpcProxy struct {
	// Mode selects which fields below are consulted; see GrpcProxyMode
	// constants. Leave empty (the zero value) to keep grpc-go's default
	// behavior — typically env-based proxy lookup.
	Mode GrpcProxyMode `yaml:"mode"`

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
	Auth *GrpcProxyAuth `yaml:"auth" default:"-"`
}

// GrpcProxyAuth carries proxy credentials. Username is required when
// the block is present; Password uses Secret so it is redacted in
// fmt/JSON/YAML/slog output.
type GrpcProxyAuth struct {
	Username string `yaml:"username"`
	Password Secret `yaml:"password"`
}

// DefaultGrpcProxy returns the zero-value GrpcProxy. Materializing it
// via ClientOptions produces no options, leaving the client on grpc-go's
// HTTPS_PROXY env default.
func DefaultGrpcProxy() GrpcProxy {
	return GrpcProxy{}
}

// Validate performs validation on the GrpcProxy configuration.
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
func (p *GrpcProxy) Validate() error {
	if p == nil {
		return nil
	}

	if err := p.validateExclusion(); err != nil {
		return err
	}

	return ValidateStruct(p,
		validation.Field(&p.Mode,
			validation.When(p.Mode != "", ozzo_rules.OneOf(grpcProxyAllowedModes...))),
		validation.Field(&p.URL,
			validation.When(p.Mode == GrpcProxyModeURL,
				validation.Required, validation.By(validateProxyURL))),
		validation.Field(&p.Host,
			validation.When(p.Mode == GrpcProxyModeHost, validation.Required)),
		validation.Field(&p.Port,
			validation.When(p.Mode == GrpcProxyModeHost,
				validation.Required, validation.Min(httpProxyMinPort), validation.Max(httpProxyMaxPort))),
		validation.Field(&p.Auth),
	)
}

// validateExclusion rejects fields that are set but ignored by the
// current Mode — those are almost always typos in the YAML.
func (p *GrpcProxy) validateExclusion() error {
	if p.Mode != GrpcProxyModeURL && p.URL != "" {
		return fmt.Errorf("GrpcProxy: url must be empty when mode is %q", p.Mode)
	}
	if p.Mode != GrpcProxyModeHost {
		if p.Host != "" {
			return fmt.Errorf("GrpcProxy: host must be empty when mode is %q", p.Mode)
		}
		if p.Port != 0 {
			return fmt.Errorf("GrpcProxy: port must be empty when mode is %q", p.Mode)
		}
		if p.Auth != nil {
			return fmt.Errorf("GrpcProxy: auth must be empty when mode is %q", p.Mode)
		}
	}
	return nil
}

// ClientOptions materializes the proxy configuration into a slice of
// transport/grpc/client options ready to be passed to grpcclient.New.
//
// A nil receiver or empty Mode returns (nil, nil) so the client keeps
// grpc-go's HTTPS_PROXY env default.
//
// An invalid URL in Mode url surfaces as an error here rather than at
// dial time.
func (p *GrpcProxy) ClientOptions() ([]grpcclient.Option, error) {
	if p == nil {
		return nil, nil
	}
	switch p.Mode {
	case "":
		return nil, nil
	case GrpcProxyModeNone:
		return []grpcclient.Option{grpcclient.WithoutProxy()}, nil
	case GrpcProxyModeURL:
		u, err := url.Parse(p.URL)
		if err != nil {
			return nil, fmt.Errorf("GrpcProxy: parse url: %w", err)
		}
		return []grpcclient.Option{grpcclient.WithProxyURL(u)}, nil
	case GrpcProxyModeHost:
		return []grpcclient.Option{grpcclient.WithProxy(p.Host, p.Port, p.userinfo())}, nil
	default:
		return nil, fmt.Errorf("GrpcProxy: unknown mode %q", p.Mode)
	}
}

// userinfo builds a *url.Userinfo from the Auth block, returning nil
// for anonymous proxies. Password is exposed (it has to be — the
// underlying URL needs the plain text) only here.
func (p *GrpcProxy) userinfo() *url.Userinfo {
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
func (a *GrpcProxyAuth) Validate() error {
	if a == nil {
		return nil
	}
	return ValidateStruct(a,
		validation.Field(&a.Username, validation.Required),
	)
}
