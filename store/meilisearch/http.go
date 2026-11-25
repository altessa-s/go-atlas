// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package meilisearch

import (
	"errors"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"unicode/utf8"

	"github.com/altessa-s/go-atlas-net/egressrule"

	coreio "github.com/altessa-s/go-atlas-core/io"
)

const (
	healthBodyLimit = 64 * 1024
	maxRedirectHops = 10
	httpPort        = "80"
	httpsPort       = "443"
)

type healthRequestKey struct{}

type healthTransport struct {
	next http.RoundTripper
}

func (t healthTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.next.RoundTrip(req)
	if err == nil && resp != nil && resp.Body != nil && req.Context().Value(healthRequestKey{}) == true {
		// The SDK reads to EOF and checks the read error before decoding JSON.
		resp.Body = coreio.NewLimitedReadCloser(resp.Body, healthBodyLimit)
	}
	return resp, err
}

func wrapHTTPClient(client *http.Client) *http.Client {
	transport := client.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	return &http.Client{
		Transport: healthTransport{next: transport},
		Jar:       client.Jar,
		Timeout:   client.Timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) > maxRedirectHops {
				return errors.New("meilisearch: redirect limit exceeded")
			}
			from, to := originOf(via[0].URL), originOf(req.URL)
			if from.host == "" || from.port == 0 || from != to {
				return errors.New("meilisearch: redirect leaves the original origin")
			}
			if client.CheckRedirect != nil {
				return client.CheckRedirect(req, via)
			}
			return nil
		},
	}
}

type origin struct {
	scheme string
	host   string
	port   uint64
}

func originOf(u *url.URL) origin {
	host := u.Hostname()
	if ip, err := netip.ParseAddr(host); err == nil {
		host = ip.String()
	} else {
		last, _ := utf8.DecodeLastRuneInString(host)
		host = egressrule.NormalizeHost(host)
		// Keep the distinction between absolute and relative DNS names.
		switch last {
		case '.', '。', '．', '｡':
			host += "."
		}
	}

	port := u.Port()
	if port == "" {
		switch u.Scheme {
		case "http":
			port = httpPort
		case "https":
			port = httpsPort
		}
	}
	number, _ := strconv.ParseUint(port, 10, 16)
	return origin{scheme: u.Scheme, host: host, port: number}
}
