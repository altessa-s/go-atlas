// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oidc

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
)

const (
	maxProviderRedirects = 10
	redirectHTTPS        = "https"
	redirectHTTP         = "http"
)

// protectedHTTPClient shares transport state but owns its redirect policy.
func protectedHTTPClient(client *http.Client, allowLoopbackHTTP bool) *http.Client {
	clone := *client
	prior := client.CheckRedirect
	clone.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) == 0 || !allowedRedirect(via[0].URL, req.URL, allowLoopbackHTTP) {
			return ErrRedirectNotAllowed
		}
		origin := *via[0].URL
		if prior != nil {
			if err := prior(req, via); err != nil {
				return err
			}
			if !allowedRedirect(&origin, req.URL, allowLoopbackHTTP) {
				return ErrRedirectNotAllowed
			}
		} else if len(via) >= maxProviderRedirects {
			return errors.New("stopped after 10 redirects")
		}
		return nil
	}
	return &clone
}

func allowedRedirect(from, to *url.URL, allowLoopbackHTTP bool) bool {
	if from == nil || to == nil || from.User != nil || to.User != nil ||
		from.Scheme != to.Scheme || !strings.EqualFold(from.Hostname(), to.Hostname()) ||
		redirectPort(from) != redirectPort(to) {
		return false
	}
	return to.Scheme == redirectHTTPS ||
		(to.Scheme == redirectHTTP && allowLoopbackHTTP && isLoopbackHost(to.Hostname()))
}

func redirectPort(u *url.URL) string {
	if port := u.Port(); port != "" {
		return port
	}
	if u.Scheme == redirectHTTPS {
		return "443"
	}
	if u.Scheme == redirectHTTP {
		return "80"
	}
	return ""
}

// prepareRevocationClients gives built-in loaders provider-local wiring before
// injection. Sharing a caller loader must not change another provider's policy.
// Third-party loaders own their HTTP behavior beyond shared-client injection.
func (p *Provider) prepareRevocationClients() {
	p.revocationLoader = p.protectedRevocationLoader(p.revocationLoader)
	if storage, ok := p.revocationStorage.(*filterRevocationStorage); ok && storage != nil {
		clone := *storage
		clone.loader = p.protectedRevocationLoader(storage.loader)
		p.revocationStorage = &clone
	}
}

func (p *Provider) protectedRevocationLoader(loader DataLoader) DataLoader {
	urlLoader, ok := loader.(*URLRevocationLoader)
	if !ok || urlLoader == nil {
		return loader
	}
	clone := *urlLoader
	if clone.Client == nil {
		clone.Client = p.client
	} else {
		clone.Client = protectedHTTPClient(clone.Client, p.opts.discoveryAllowLoopbackHTTP)
	}
	return &clone
}
