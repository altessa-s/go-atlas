// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package validators

import (
	"errors"
	"net"
	"net/netip"
	"net/url"
	"strings"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// CIDRPrefix requires an IPv4 or IPv6 prefix with an explicit prefix length.
var CIDRPrefix = validation.By(func(value any) error {
	raw, _ := value.(string)
	_, err := netip.ParsePrefix(raw)
	return coreerrs.Wrap(err, "must be a valid CIDR prefix (e.g. 10.0.0.0/8)")
})

// HTTPSURL accepts an optional absolute HTTPS endpoint. allowHTTP permits HTTP
// on any host. allowLoopbackHTTP permits HTTP on localhost and loopback IPs.
func HTTPSURL(allowHTTP, allowLoopbackHTTP bool) validation.Rule {
	return validation.By(func(value any) error {
		raw, ok := value.(string)
		if ok && raw == "" {
			return nil
		}
		u, err := url.Parse(raw)
		if err != nil || u.Host == "" {
			return errors.New("must be an absolute https:// URL")
		}
		if u.Scheme == "https" {
			return nil
		}
		if u.Scheme == "http" {
			host := u.Hostname()
			if allowHTTP || (allowLoopbackHTTP && (strings.EqualFold(host, "localhost") || net.ParseIP(host).IsLoopback())) {
				return nil
			}
		}
		return errors.New("must use https:// (set allowInsecureHTTP to permit http://)")
	})
}
