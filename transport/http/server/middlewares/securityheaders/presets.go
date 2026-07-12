// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package securityheaders

// RecommendedPermissionsPolicy denies the commonly-abused powerful browser
// features. It is safe for typical applications — ordinary pages do not use these
// features — while blocking silent access to location, camera, microphone, etc.
// Override with [WithPermissionsPolicy] if your app legitimately needs a feature.
const RecommendedPermissionsPolicy = "geolocation=(), microphone=(), camera=(), payment=(), usb=(), magnetometer=(), gyroscope=(), accelerometer=()"

// RecommendedOptions returns an opt-in bundle of safe hardening options: it pins
// the non-breaking defaults (X-Frame-Options: DENY, a strict Referrer-Policy,
// X-Content-Type-Options: nosniff, X-XSS-Protection: 0) and adds a conservative
// Permissions-Policy. Splat it into [New]:
//
//	mw := securityheaders.New(securityheaders.RecommendedOptions()...)
//
// It deliberately does NOT set Content-Security-Policy, Strict-Transport-Security,
// or any Cross-Origin-*-Policy header: those are application- and
// deployment-specific and can break a working service if imposed blindly. Set
// them explicitly (WithContentSecurityPolicy, WithHstsEnabled, etc.) once you have
// validated a policy for your app. Append your own options after the preset to
// override any entry.
func RecommendedOptions() []Option {
	return []Option{
		WithFrameOptions(FrameOptionsDeny),
		WithReferrerPolicy(ReferrerPolicyStrictOriginWhenCrossOrigin),
		WithContentTypeNoSniff(),
		WithXssProtectionDisabled(),
		WithPermissionsPolicy(RecommendedPermissionsPolicy),
	}
}

// StrictOptions returns [RecommendedOptions] plus the softer cross-origin
// isolation headers: Cross-Origin-Opener-Policy: same-origin and
// Cross-Origin-Resource-Policy: same-origin. These reduce cross-window and
// cross-origin-embedding exposure and are safe for most single-origin apps, but
// can affect OAuth popups and cross-origin embedding — validate before use.
//
// It still does NOT set Cross-Origin-Embedder-Policy (require-corp is the most
// disruptive header and must be enabled explicitly with
// [WithCrossOriginEmbedderPolicy]), nor CSP/HSTS. Append your own options after
// the preset to override any entry.
func StrictOptions() []Option {
	return append(RecommendedOptions(),
		WithCrossOriginOpenerPolicy(COOPSameOrigin),
		WithCrossOriginResourcePolicy(CORPSameOrigin),
	)
}
