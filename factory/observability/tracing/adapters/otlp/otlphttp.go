// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"fmt"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"

	"github.com/altessa-s/go-atlas-core/collections/slices"

	httpclient "github.com/altessa-s/go-atlas-net/http/client"
	observabilityconfig "github.com/altessa-s/go-atlas/config/observability"
	proxyconfig "github.com/altessa-s/go-atlas/config/proxy"
	httpclientfactory "github.com/altessa-s/go-atlas/factory/ext/net/httpclient"
)

// Default destination ports of the URL schemes an OTLP collector or its
// proxy is reached with.
const (
	defaultHTTPPort   = 80
	defaultHTTPSPort  = 443
	defaultSOCKS5Port = 1080
)

// Egress rule names reported when the policy decides a destination.
const (
	collectorRuleName = "otlpCollector"
	proxyRuleName     = "otlpProxy"
)

var (
	anyIPv4 = netip.MustParsePrefix("0.0.0.0/0")
	anyIPv6 = netip.MustParsePrefix("::/0")
)

// newOTLPHTTPClient builds the resilient client of the OTLP/HTTP exporter.
//
// SSRF protection stays on. The collector is an operator-configured endpoint,
// often on a private network, so the client's egress policy allows exactly
// its host and port: a literal IP as that address, a host name at whatever
// address it resolves to when dialed. The proxy that carries the export, when
// one applies, is allowed the same way. Every other private, loopback,
// link-local or reserved destination is refused with an error matching
// [httpclient.ErrSSRFBlocked].
//
// Redirects are not followed: the policy admits any public destination, so a
// redirect from a compromised collector (or a MITM on an insecure endpoint)
// would otherwise replay the export, custom API-key headers included, to a
// host of its choosing. A 3xx reply reaches the exporter as a failed export.
//
// Without a proxy block the environment proxies apply, as for grpc-go. Retries
// stay with otlptracehttp, which honors the OTLP retry semantics.
func newOTLPHTTPClient(cfg *observabilityconfig.TracingOTLP) (*http.Client, error) {
	proxyOpts, err := httpclientfactory.ProxyOptions(cfg.Proxy)
	if err != nil {
		return nil, fmt.Errorf("failed to materialize OTLP proxy options: %w", err)
	}
	collector, err := otlpCollectorURL(cfg)
	if err != nil {
		return nil, err
	}
	allow, err := endpointAllowRules(collectorRuleName, collector)
	if err != nil {
		return nil, err
	}

	proxy := http.ProxyFromEnvironment
	if len(proxyOpts) > 0 {
		proxy = nil // the proxy options install the configured resolver
	}
	proxyURL, err := otlpProxyURL(cfg.Proxy, collector)
	if err != nil {
		return nil, err
	}
	if proxyURL != nil {
		rules, rerr := endpointAllowRules(proxyRuleName, proxyURL)
		if rerr != nil {
			return nil, fmt.Errorf("OTLP proxy: %w", rerr)
		}
		allow = append(allow, rules...)
	}

	policy, err := httpclient.NewEgressPolicy(allow, nil)
	if err != nil {
		return nil, err
	}
	opts := []httpclient.Option{httpclient.WithEgressPolicy(policy), httpclient.WithoutRetry(), httpclient.WithoutRedirects()}
	opts = slices.AppendIf(opts, proxy != nil, httpclient.WithProxyFunc(proxy))
	return httpclient.New(append(opts, proxyOpts...)...), nil
}

// otlpCollectorURL returns the URL of the collector: the exporter sends to
// the host[:port] endpoint over http when insecure, https otherwise.
func otlpCollectorURL(cfg *observabilityconfig.TracingOTLP) (*url.URL, error) {
	scheme := "https"
	if cfg.Insecure {
		scheme = "http"
	}
	u := &url.URL{Scheme: scheme, Host: cfg.Endpoint, Path: "/v1/traces"}
	if _, _, err := endpointHostPort(u); err != nil {
		return nil, fmt.Errorf("OTLP endpoint %q: want host[:port]: %w", cfg.Endpoint, err)
	}
	return u, nil
}

// otlpProxyURL returns the proxy the export to collector goes through, or nil
// when it is sent directly: the configured proxy, or without a proxy block the
// environment proxy for the collector URL.
func otlpProxyURL(p *proxyconfig.Config, collector *url.URL) (*url.URL, error) {
	if p == nil || p.Mode == "" {
		return http.ProxyFromEnvironment(&http.Request{URL: collector})
	}
	return p.URLValue()
}

// endpointAllowRules returns the egress allow rules admitting exactly the
// host and port of u: a literal IP as that address, a host name resolved to
// any address.
func endpointAllowRules(name string, u *url.URL) ([]httpclient.EgressRule, error) {
	host, port, err := endpointHostPort(u)
	if err != nil {
		return nil, err
	}
	if ip, err := netip.ParseAddr(host); err == nil {
		ip = ip.WithZone("")
		return []httpclient.EgressRule{{Name: name, CIDR: netip.PrefixFrom(ip, ip.BitLen()), Port: port}}, nil
	}
	return []httpclient.EgressRule{
		{Name: name, Host: host, CIDR: anyIPv4, Port: port},
		{Name: name, Host: host, CIDR: anyIPv6, Port: port},
	}, nil
}

// endpointHostPort returns the host of u and its explicit port, or the
// default port of its scheme. The host must be a plain name or IP address:
// glob metacharacters would widen the allow rule built from it.
func endpointHostPort(u *url.URL) (string, uint16, error) {
	host, portText := u.Hostname(), u.Port()
	canonical := host
	if strings.Contains(host, ":") {
		canonical = "[" + host + "]"
	}
	if portText != "" {
		canonical += ":" + portText
	}
	if host == "" || canonical != u.Host || strings.ContainsAny(host, `*?[]\`) {
		return "", 0, fmt.Errorf("invalid host:port %q", u.Host)
	}
	if portText == "" {
		switch u.Scheme {
		case "https":
			return host, defaultHTTPSPort, nil
		case "socks5", "socks5h":
			return host, defaultSOCKS5Port, nil
		default:
			return host, defaultHTTPPort, nil
		}
	}
	port, err := strconv.ParseUint(portText, 10, 16)
	if err != nil || port == 0 {
		return "", 0, fmt.Errorf("invalid port %q", portText)
	}
	return host, uint16(port), nil
}
