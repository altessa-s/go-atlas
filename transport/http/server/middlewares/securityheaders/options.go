// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package securityheaders

//go:generate go run github.com/altessa-s/go-atlas/cmd/optgen generate --type=options

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

// CrossOriginOpenerPolicy represents the Cross-Origin-Opener-Policy header value.
// COOP isolates the browsing context group; it can break OAuth popups and other
// cross-window integrations, so it is off by default and opt-in only.
type CrossOriginOpenerPolicy string

const (
	// COOPSameOrigin isolates the document to same-origin documents.
	COOPSameOrigin CrossOriginOpenerPolicy = "same-origin"
	// COOPSameOriginAllowPopups keeps isolation but preserves references to popups
	// it opens — the safer choice when the app opens OAuth/consent popups.
	COOPSameOriginAllowPopups CrossOriginOpenerPolicy = "same-origin-allow-popups"
	// COOPUnsafeNone disables isolation (the browser default).
	COOPUnsafeNone CrossOriginOpenerPolicy = "unsafe-none"
)

// CrossOriginEmbedderPolicy represents the Cross-Origin-Embedder-Policy header
// value. COEP=require-corp is the most disruptive security header — it blocks any
// cross-origin subresource lacking CORP/CORS — so it is off by default and must
// be enabled explicitly, never via a preset.
type CrossOriginEmbedderPolicy string

const (
	// COEPRequireCorp requires every cross-origin subresource to opt in via CORP
	// or CORS. Enables cross-origin isolation but commonly breaks third-party
	// resources; enable only after auditing every embedded resource.
	COEPRequireCorp CrossOriginEmbedderPolicy = "require-corp"
	// COEPCredentialless loads cross-origin subresources without credentials
	// instead of blocking them — a softer alternative to require-corp.
	COEPCredentialless CrossOriginEmbedderPolicy = "credentialless" // #nosec G101 -- COEP header value, not a credential
	// COEPUnsafeNone disables embedder policy (the browser default).
	COEPUnsafeNone CrossOriginEmbedderPolicy = "unsafe-none"
)

// CrossOriginResourcePolicy represents the Cross-Origin-Resource-Policy header
// value, which controls who may embed this response as a subresource.
type CrossOriginResourcePolicy string

const (
	// CORPSameOrigin allows only same-origin documents to embed the resource.
	CORPSameOrigin CrossOriginResourcePolicy = "same-origin"
	// CORPSameSite allows same-site documents to embed the resource.
	CORPSameSite CrossOriginResourcePolicy = "same-site"
	// CORPCrossOrigin allows any origin to embed the resource.
	CORPCrossOrigin CrossOriginResourcePolicy = "cross-origin"
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

	// crossOriginOpenerPolicy sets the Cross-Origin-Opener-Policy header.
	// Empty string (default) means the header is not set. Opt-in: COOP can break
	// cross-window integrations (OAuth popups, embedding).
	crossOriginOpenerPolicy CrossOriginOpenerPolicy

	// crossOriginEmbedderPolicy sets the Cross-Origin-Embedder-Policy header.
	// Empty string (default) means the header is not set. Opt-in and disruptive:
	// require-corp blocks cross-origin subresources without CORP/CORS.
	crossOriginEmbedderPolicy CrossOriginEmbedderPolicy

	// crossOriginResourcePolicy sets the Cross-Origin-Resource-Policy header.
	// Empty string (default) means the header is not set.
	crossOriginResourcePolicy CrossOriginResourcePolicy
}
