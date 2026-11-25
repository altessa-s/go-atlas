// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package dnsresolver

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strings"
)

const (
	// defaultPort is the port of a target that names none.
	defaultPort = "443"
	// defaultDNSPort is the port of a DNS server authority that names none.
	defaultDNSPort = "53"
)

var (
	// errMissingAddress rejects an empty target or authority.
	errMissingAddress = errors.New("missing address")
	// errMissingPort rejects a target that ends with the host and port separator.
	errMissingPort = errors.New("missing port after port-separator colon")
)

// parseTarget splits a target into host and port the way grpc-go's dns
// resolver does: a bare IPv4 or IPv6 address or a host without a port gets
// defaultPort, brackets around an IPv6 host are removed, and ":port" means
// localhost (as with [net.Dial]). A trailing colon or bracketed empty host
// is an error.
func parseTarget(target, defaultPort string) (host, port string, err error) {
	if target == "" {
		return "", "", errMissingAddress
	}
	if isIP(target) {
		return target, defaultPort, nil
	}
	if host, port, err = net.SplitHostPort(target); err == nil {
		if port == "" {
			return "", "", errMissingPort
		}
		if host == "" {
			if !strings.HasPrefix(target, ":") {
				return "", "", errMissingAddress
			}
			host = "localhost"
		}
		return host, port, nil
	}
	if host, port, err = net.SplitHostPort(target + ":" + defaultPort); err == nil {
		if host == "" {
			return "", "", errMissingAddress
		}
		return host, port, nil
	}
	return "", "", fmt.Errorf("invalid address %q: %w", target, err)
}

// isIP reports whether host is an IPv4 or IPv6 address literal.
func isIP(host string) bool {
	_, err := netip.ParseAddr(host)
	return err == nil
}

// dnsServer returns the host:port of the DNS server an authority names, or ""
// for an empty authority, which selects the system resolver.
func dnsServer(authority string) (string, error) {
	if authority == "" {
		return "", nil
	}
	host, port, err := parseTarget(authority, defaultDNSPort)
	if err != nil {
		return "", err
	}
	return net.JoinHostPort(host, port), nil
}

// netResolver returns the resolver that queries server, or the system
// DNS configuration for an empty server. Use the Go resolver so StrictErrors
// preserves temporary errors during search-domain resolution.
func netResolver(server string) *net.Resolver {
	r := &net.Resolver{PreferGo: true, StrictErrors: true}
	if server != "" {
		r.Dial = func(ctx context.Context, network, _ string) (net.Conn, error) {
			var dialer net.Dialer
			return dialer.DialContext(ctx, network, server)
		}
	}
	return r
}
