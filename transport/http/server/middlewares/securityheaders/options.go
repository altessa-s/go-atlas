// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package securityheaders

//go:generate go run github.com/altessa-s/go-atlas/tools/codegen/optgen generate --type=options

import (
	"log/slog"
	"regexp"

	"github.com/altessa-s/go-atlas/transport/http/server/middlewares/defaults"
)

// Use defaults package for optgen code generation
var _ = defaults.IgnorePatterns

// FrameOptions represents the X-Frame-Options header value.
type FrameOptions string

const (
	// FrameOptionsDeny prevents the page from being displayed in a frame.
	FrameOptionsDeny FrameOptions = "DENY"

	// FrameOptionsSameOrigin allows the page to be displayed in a frame on the same origin.
	FrameOptionsSameOrigin FrameOptions = "SAMEORIGIN"
)

// ReferrerPolicy represents the Referrer-Policy header value.
type ReferrerPolicy string

const (
	// ReferrerPolicyNoReferrer sends no referrer information.
	ReferrerPolicyNoReferrer ReferrerPolicy = "no-referrer"

	// ReferrerPolicyNoReferrerWhenDowngrade is the default browser behavior.
	ReferrerPolicyNoReferrerWhenDowngrade ReferrerPolicy = "no-referrer-when-downgrade"

	// ReferrerPolicySameOrigin sends referrer only for same-origin requests.
	ReferrerPolicySameOrigin ReferrerPolicy = "same-origin"

	// ReferrerPolicyOrigin sends only the origin, not the full URL.
	ReferrerPolicyOrigin ReferrerPolicy = "origin"

	// ReferrerPolicyStrictOrigin sends origin only for same-protocol requests.
	ReferrerPolicyStrictOrigin ReferrerPolicy = "strict-origin"

	// ReferrerPolicyOriginWhenCrossOrigin sends full URL for same-origin, origin for cross-origin.
	ReferrerPolicyOriginWhenCrossOrigin ReferrerPolicy = "origin-when-cross-origin"

	// ReferrerPolicyStrictOriginWhenCrossOrigin is the recommended default.
	// Sends full URL for same-origin, origin for cross-origin same-protocol, nothing for downgrade.
	ReferrerPolicyStrictOriginWhenCrossOrigin ReferrerPolicy = "strict-origin-when-cross-origin"

	// ReferrerPolicyUnsafeURL sends full URL for all requests (not recommended).
	ReferrerPolicyUnsafeURL ReferrerPolicy = "unsafe-url"
)

// options configures the security headers middleware.
type options struct {
	logger         *slog.Logger
	ignorePaths    []string
	ignorePatterns []*regexp.Regexp `optgen:"default=defaults.IgnorePatterns"`

	// frameOptions controls the X-Frame-Options header.
	// Default: DENY
	frameOptions FrameOptions `optgen:"default=FrameOptionsDeny"`

	// referrerPolicy controls the Referrer-Policy header.
	// Default: strict-origin-when-cross-origin
	referrerPolicy ReferrerPolicy `optgen:"default=ReferrerPolicyStrictOriginWhenCrossOrigin"`

	// contentSecurityPolicy sets the Content-Security-Policy header.
	// Empty string means the header is not set.
	contentSecurityPolicy string

	// permissionsPolicy sets the Permissions-Policy header.
	// Empty string means the header is not set.
	permissionsPolicy string

	// hstsEnabled enables the Strict-Transport-Security header.
	// Only enable this if your server is accessed exclusively via HTTPS.
	hstsEnabled bool

	// hstsMaxAge is the max-age value for HSTS in seconds.
	// Default: 31536000 (1 year)
	hstsMaxAge int `optgen:"default=31536000"`

	// hstsIncludeSubDomains includes subdomains in HSTS policy.
	// Default: true
	hstsIncludeSubDomains bool `optgen:"default=true"`

	// hstsPreload enables the preload directive for HSTS.
	// Only enable if you want to submit your domain to browser preload lists.
	// See: https://hstspreload.org/
	hstsPreload bool

	// contentTypeNoSniff controls whether X-Content-Type-Options: nosniff is set.
	// Default: true
	contentTypeNoSniff bool `optgen:"default=true"`

	// xssProtectionDisabled controls whether X-XSS-Protection: 0 is set.
	// Setting this to 0 is recommended as browser XSS filters can introduce vulnerabilities.
	// Default: true (header is set to 0)
	xssProtectionDisabled bool `optgen:"default=true"`
}
