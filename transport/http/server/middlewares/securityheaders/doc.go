// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package securityheaders provides middleware for adding security-related HTTP headers.
//
// This middleware adds headers that help protect against common web vulnerabilities
// including XSS, clickjacking, MIME sniffing attacks, and more.
//
// Headers set by default:
//   - X-Content-Type-Options: nosniff (prevents MIME type sniffing)
//   - X-Frame-Options: DENY (prevents clickjacking)
//   - Referrer-Policy: strict-origin-when-cross-origin (controls referrer information)
//   - X-XSS-Protection: 0 (disabled, as modern browsers have built-in XSS protection)
//
// Optional headers (configurable):
//   - Strict-Transport-Security (HSTS): enforces HTTPS connections
//   - Content-Security-Policy (CSP): controls resource loading
//   - Permissions-Policy: controls browser features
//
// Example usage:
//
//	// Basic usage with defaults
//	mw := securityheaders.Middleware()
//	handler := mw(yourHandler)
//
//	// With HSTS enabled (for production HTTPS servers)
//	mw := securityheaders.Middleware(
//	    securityheaders.WithHstsEnabled(),
//	    securityheaders.WithHstsMaxAge(31536000), // 1 year
//	)
//
//	// With custom Content-Security-Policy
//	mw := securityheaders.Middleware(
//	    securityheaders.WithContentSecurityPolicy("default-src 'self'"),
//	)
//
//	// With custom frame options (allow same origin framing)
//	mw := securityheaders.Middleware(
//	    securityheaders.WithFrameOptions(securityheaders.FrameOptionsSameOrigin),
//	)
//
// OWASP References:
//   - https://owasp.org/www-project-secure-headers/
//   - A05:2021 - Security Misconfiguration
package securityheaders
