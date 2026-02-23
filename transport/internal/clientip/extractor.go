// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package clientip

import (
	"context"
	"iter"
	"log/slog"
	"net/netip"
	"slices"
	"strings"

	"github.com/altessa-s/go-atlas/data/cache/lru"

	coreslices "github.com/altessa-s/go-atlas/core/collections/slices"
	coreerrs "github.com/altessa-s/go-atlas/core/errors"
	corestrings "github.com/altessa-s/go-atlas/core/text/strings"
)

// HeaderGetter abstracts multi-valued header access so that [Extractor] can
// work with both HTTP requests (via [net/http.Header.Values]) and gRPC
// incoming metadata without depending on either transport directly.
type HeaderGetter interface {
	// GetHeader returns all values for the given header name.
	GetHeader(name string) []string
}

// Extractor resolves the real client IP from a request's peer address and
// proxy headers. It consults headers in a deterministic order, applies
// trusted-proxy and private-range filtering, and caches parsed addresses
// in an LRU to reduce allocation pressure under load.
//
// Safe for concurrent use; the underlying LRU cache is thread-safe.
type Extractor struct {
	opts    *options
	cache   lru.Cacher[string, netip.Addr]
	headers []string // Interned headers for performance
}

// NewExtractor creates a new [Extractor] configured with the given options.
// Returns an error if the LRU cache cannot be initialized (e.g. invalid size).
//
// Example:
//
//	extractor, err := clientip.NewExtractor(
//		clientip.WithTrustedProxies(netip.MustParsePrefix("10.0.0.0/8")),
//	)
func NewExtractor(opt ...Option) (*Extractor, error) {
	opts := newOptions(opt...)

	var cache lru.Cacher[string, netip.Addr]

	// Only create cache if not disabled and size > 0
	if !opts.cacheDisabled && opts.cacheSize > 0 {
		var err error
		if cache, err = lru.NewCache[string, netip.Addr](opts.cacheSize); err != nil {
			return nil, coreerrs.Wrapf(err, "failed to create IP cache, size=%d", opts.cacheSize)
		}
	}

	return &Extractor{
		opts:    opts,
		cache:   cache,
		headers: corestrings.InternStringSlice(opts.headers),
	}, nil
}

// Extract determines the real client IP using the following strategy:
//
//  1. If peerIP is neither trusted nor private, it is returned immediately.
//  2. Otherwise, configured proxy headers are walked in order; for
//     X-Forwarded-For the rightmost non-trusted, non-private entry wins.
//  3. If no public IP is found in any header, peerIP is returned as a
//     fallback (covers local development with loopback addresses).
//
// Returns an invalid [netip.Addr] only when peerIP itself is invalid and
// no header yields a valid address.
func (e *Extractor) Extract(ctx context.Context, peerIP netip.Addr, headers HeaderGetter) netip.Addr {
	e.opts.logger.DebugContext(ctx, "peer IP",
		slog.String("ip", peerIP.String()),
		slog.Bool("is_private", IsPrivate(peerIP)),
		slog.Bool("is_trusted", IsTrusted(peerIP, e.opts.trustedPeers)),
	)

	if !IsTrusted(peerIP, e.opts.trustedPeers) && !IsPrivate(peerIP) {
		e.opts.logger.DebugContext(ctx, "using peer IP as real IP", slog.String("ip", peerIP.String()))
		return peerIP
	}

	// Process headers only if enabled
	if !e.opts.headersEnabled {
		e.opts.logger.DebugContext(ctx, "headers processing disabled, falling back to peer IP",
			slog.String("ip", peerIP.String()))
		return peerIP
	}

	totalProcessed := 0
	// Process headers in deterministic order
	for _, header := range e.headers {
		if totalProcessed >= MaxTotalIPs {
			break
		}

		ips := e.getIPsFromHeader(ctx, header, headers)
		if len(ips) == 0 {
			continue
		}

		if header == InternedHeaderXForwardedFor {
			if ip := e.parseXForwardedForHeader(ctx, ips); ip.IsValid() {
				e.opts.logger.DebugContext(ctx, "found IP from X-Forwarded-For",
					slog.String("ip", ip.String()))
				return ip
			}
		}

		for _, ip := range ips {
			if totalProcessed >= MaxTotalIPs {
				e.opts.logger.DebugContext(ctx, "reached total IP limit", slog.Int("limit", MaxTotalIPs))
				break
			}
			totalProcessed++

			if addr, ok := e.parseAndCheckIP(ctx, ip, header); ok {
				return addr
			}
		}
	}

	// No valid IP found from headers — fall back to the peer IP.
	// This handles local development (127.0.0.1, ::1) where no proxy headers are present.
	e.opts.logger.DebugContext(ctx, "no IP from headers, falling back to peer IP",
		slog.String("ip", peerIP.String()))

	return peerIP
}

func (e *Extractor) getIPsFromHeader(ctx context.Context, header string, headers HeaderGetter) []string {
	headerValues := headers.GetHeader(header)
	if len(headerValues) == 0 {
		return nil
	}

	e.opts.logger.DebugContext(ctx, "processing header",
		slog.String("header", header), slog.Int("values_count", len(headerValues)))

	ips := make([]string, 0, len(headerValues))
	for _, headerValue := range headerValues {
		// Limit header value length for safety
		if len(headerValue) > MaxIPLength*MaxIPsPerHeader*2 {
			e.opts.logger.WarnContext(ctx, "header value too long, skipping",
				slog.String("header", header),
				slog.Int("length", len(headerValue)))
			continue
		}

		// Split and trim using SplitSeq for efficiency
		for part := range strings.SplitSeq(headerValue, ",") {
			trimmed := corestrings.InternTrimString(part)
			if trimmed != "" && len(trimmed) <= MaxIPLength {
				ips = append(ips, trimmed)
				if len(ips) >= MaxIPsPerHeader {
					break
				}
			}
		}

		if len(ips) >= MaxIPsPerHeader {
			break
		}
	}

	// Apply per-header limit
	originalCount := len(ips)
	if len(ips) > MaxIPsPerHeader {
		ips = ips[:MaxIPsPerHeader]
		e.opts.logger.DebugContext(ctx, "truncated IPs to limit",
			slog.String("header", header),
			slog.Int("original", originalCount),
			slog.Int("truncated", len(ips)))
	}

	if len(ips) > 0 {
		e.opts.logger.DebugContext(ctx, "ips from header", slog.String("header", header),
			slog.Any("ips", ips))
	}

	return ips
}

func (e *Extractor) parseAndCheckIP(ctx context.Context, ip, header string) (netip.Addr, bool) {
	// Check cache first if available
	if e.cache != nil {
		if cachedAddr, ok := e.cache.Get(ip); ok {
			e.opts.logger.DebugContext(ctx, "found in cache",
				slog.String("ip", cachedAddr.String()),
				slog.Bool("is_private", IsPrivate(cachedAddr)))

			if !IsPrivate(cachedAddr) {
				return cachedAddr, true
			}

			return netip.Addr{}, false
		}
	}

	// Intern the IP string for better memory efficiency
	internedIP := corestrings.InternString(ip)

	addr, err := netip.ParseAddr(internedIP)
	if err != nil {
		e.opts.logger.WarnContext(ctx, "failed to parse IP",
			slog.String("ip", internedIP),
			slog.String("error", err.Error()))
		return netip.Addr{}, false
	}

	// Cache the parsed IP if cache is available
	if e.cache != nil {
		e.cache.Put(internedIP, addr)
	}

	e.opts.logger.DebugContext(ctx, "parsed IP",
		slog.String("ip", addr.String()),
		slog.Bool("is_private", IsPrivate(addr)))

	if !IsPrivate(addr) {
		e.opts.logger.DebugContext(ctx, "found public IP",
			slog.String("ip", addr.String()),
			slog.String("header", header))
		return addr, true
	}

	return netip.Addr{}, false
}

// Headers returns an iterator over the configured header names in their
// evaluation order.
func (e *Extractor) Headers() iter.Seq[string] {
	return slices.Values(e.headers)
}

// parseXForwardedForHeader parses X-Forwarded-For header values.
func (e *Extractor) parseXForwardedForHeader(ctx context.Context, ips []string) netip.Addr {
	if len(ips) == 0 {
		return netip.Addr{}
	}

	e.opts.logger.DebugContext(ctx, "parsing X-Forwarded-For",
		slog.Any("ips", ips),
		slog.Uint64("trusted_proxies_count", uint64(e.opts.trustedProxiesCount)))

	// X-Forwarded-For format: client, proxy1, proxy2, ...
	// We need to find the rightmost IP that is not from a trusted proxy

	// If we have configured trusted proxies count, we can trust that many proxies from the right
	if e.opts.trustedProxiesCount > 0 && uint(len(ips)) > e.opts.trustedProxiesCount {
		index := len(ips) - int(e.opts.trustedProxiesCount) - 1 // #nosec G115 -- proxy count is small
		if index >= 0 && index < len(ips) {
			ip := ips[index]
			e.opts.logger.DebugContext(ctx, "checking IP by trusted proxy count",
				slog.String("ip", ip),
				slog.Int("index", index))

			addr, err := netip.ParseAddr(ip)
			if err == nil && !IsPrivate(addr) {
				e.opts.logger.DebugContext(ctx, "found IP by trusted proxy count",
					slog.String("ip", addr.String()))
				return addr
			}
		}
	}

	// Otherwise, iterate from right to left and find the first non-private, non-trusted IP
	for _, ip := range coreslices.Backward(ips) {
		if len(ip) > MaxIPLength {
			continue
		}

		// Intern the IP string for better memory efficiency
		internedIP := corestrings.InternString(ip)

		// Check cache first if available
		if e.cache != nil {
			if cachedAddr, ok := e.cache.Get(internedIP); ok {
				if !IsTrusted(cachedAddr, e.opts.trustedProxies) && !IsPrivate(cachedAddr) {
					return cachedAddr
				}
				continue
			}
		}

		addr, err := netip.ParseAddr(internedIP)
		if err != nil {
			continue
		}

		// Cache the parsed IP if cache is available
		if e.cache != nil {
			e.cache.Put(internedIP, addr)
		}

		// Skip if this is a trusted proxy or private IP
		if IsTrusted(addr, e.opts.trustedProxies) || IsPrivate(addr) {
			continue
		}

		// Found a public, untrusted IP - this is likely the real client IP
		return addr
	}

	// If all IPs are private or trusted, return the leftmost one (original client)
	if len(ips) > 0 {
		internedIP := corestrings.InternString(ips[0])
		addr, err := netip.ParseAddr(internedIP)
		if err == nil {
			return addr
		}
	}

	return netip.Addr{}
}
