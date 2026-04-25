// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package cors provides middleware for handling Cross-Origin Resource Sharing (CORS).
//
// CORS is a security feature implemented by browsers that restricts web pages from
// making requests to a different domain than the one that served the original page.
// This middleware adds the appropriate headers to allow cross-origin requests.
//
// Headers managed:
//   - Access-Control-Allow-Origin: specifies which origins can access the resource
//   - Access-Control-Allow-Methods: specifies allowed HTTP methods
//   - Access-Control-Allow-Headers: specifies allowed request headers
//   - Access-Control-Allow-Credentials: indicates if credentials are allowed
//   - Access-Control-Expose-Headers: specifies which headers can be exposed
//   - Access-Control-Max-Age: indicates how long preflight results can be cached
//
// Example usage:
//
//	// Allow specific origin
//	mw := cors.Middleware(
//	    cors.WithAllowedOrigins("https://example.com"),
//	)
//
//	// Allow multiple origins
//	mw := cors.Middleware(
//	    cors.WithAllowedOrigins("https://example.com", "https://app.example.com"),
//	)
//
//	// Allow all origins (use with caution)
//	mw := cors.Middleware(
//	    cors.WithAllowAllOrigins(),
//	)
//
//	// Full configuration
//	mw := cors.Middleware(
//	    cors.WithAllowedOrigins("https://example.com"),
//	    cors.WithAllowedMethods("GET", "POST", "PUT", "DELETE"),
//	    cors.WithAllowedHeaders("Content-Type", "Authorization"),
//	    cors.WithAllowCredentials(),
//	    cors.WithMaxAge(86400),
//	)
//
// Security considerations:
//   - Avoid using AllowAllOrigins with AllowCredentials as this is insecure
//   - Be specific about allowed origins in production environments
//   - Consider limiting exposed headers to only what's necessary
//
// References:
//   - https://developer.mozilla.org/en-US/docs/Web/HTTP/CORS
//   - https://fetch.spec.whatwg.org/#http-cors-protocol
package cors
