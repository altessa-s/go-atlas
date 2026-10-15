// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package clientip

import (
	"context"
	"iter"
	"log/slog"
	"net/http"
	"net/netip"
	"slices"
	"strings"

	"github.com/altessa-s/go-atlas/data/cache/lru"

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

// HTTPHeader adapts an [http.Header] to [HeaderGetter]:
// extractor.Extract(ctx, peerIP, clientip.HTTPHeader(r.Header)).
type HTTPHeader http.Header

// GetHeader returns all values for the given header name.
func (h HTTPHeader) GetHeader(name string) []string {
	return http.Header(h).Values(name)
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
	const mappedIPv4PrefixBits = 96

	opts := newOptions(opt...)
	// Addresses are unmapped before trust checks, so mapped IPv4 prefixes
	// must use the same representation. Clone to avoid changing caller slices.
	for _, prefixes := range []*[]netip.Prefix{&opts.trustedPeers, &opts.trustedProxies} {
		*prefixes = slices.Clone(*prefixes)
		for i, prefix := range *prefixes {
			if prefix.Addr().Is4In6() && prefix.Bits() >= mappedIPv4PrefixBits {
				(*prefixes)[i] = netip.PrefixFrom(prefix.Addr().Unmap(), prefix.Bits()-mappedIPv4PrefixBits)
			}
		}
	}

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

// debugEnabled reports whether debug logs are emitted. Debug calls are guarded
// so disabled logging does not format addresses on every extraction.
func (e *Extractor) debugEnabled(ctx context.Context) bool {
	return e.opts.logger.Enabled(ctx, slog.LevelDebug)
}

// Extract determines the real client IP using the following strategy:
//
//  1. If peerIP is not trusted — not in the trusted peers or trusted proxies,
//     and not private with [WithTrustPrivatePeers] set — it is returned
//     immediately: an untrusted peer cannot speak for anyone else.
//  2. Otherwise, configured proxy headers are walked in order; for
//     X-Forwarded-For the rightmost non-trusted, non-private entry wins,
//     or a nonzero trusted proxy count selects an exact position.
//  3. If no public IP is found in any header, peerIP is returned as a
//     fallback.
//
// Returns an invalid [netip.Addr] only when peerIP itself is invalid and
// no header yields a valid address.
func (e *Extractor) Extract(ctx context.Context, peerIP netip.Addr, headers HeaderGetter) netip.Addr {
	trusted := e.peerTrusted(peerIP.Unmap())

	if e.debugEnabled(ctx) {
		e.opts.logger.DebugContext(ctx, "peer IP",
			slog.String("ip", peerIP.String()),
			slog.Bool("is_private", IsPrivate(peerIP.Unmap())),
			slog.Bool("is_trusted", trusted),
		)
	}

	if !trusted {
		if e.debugEnabled(ctx) {
			e.opts.logger.DebugContext(ctx, "using peer IP as real IP", slog.String("ip", peerIP.String()))
		}
		return peerIP
	}

	// Process headers only if enabled
	if !e.opts.headersEnabled {
		if e.debugEnabled(ctx) {
			e.opts.logger.DebugContext(ctx, "headers processing disabled, falling back to peer IP",
				slog.String("ip", peerIP.String()))
		}
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

		if isForwardedFor(header) {
			if ip := e.parseXForwardedForHeader(ctx, ips); ip.IsValid() {
				if e.debugEnabled(ctx) {
					e.opts.logger.DebugContext(ctx, "found IP from X-Forwarded-For",
						slog.String("ip", ip.String()))
				}
				return ip
			}
			if e.opts.trustedProxiesCount > 0 {
				// An unverifiable count-selected client must not be replaced
				// by a claim from another forwarding header.
				return peerIP
			}
			// The XFF parser already rejected every entry as private or a
			// trusted proxy; the generic parser below would accept a public
			// trusted proxy, so XFF is never reprocessed through it.
			totalProcessed += len(ips)
			continue
		}

		for _, ip := range ips {
			if totalProcessed >= MaxTotalIPs {
				if e.debugEnabled(ctx) {
					e.opts.logger.DebugContext(ctx, "reached total IP limit", slog.Int("limit", MaxTotalIPs))
				}
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
	if e.debugEnabled(ctx) {
		e.opts.logger.DebugContext(ctx, "no IP from headers, falling back to peer IP",
			slog.String("ip", peerIP.String()))
	}

	return peerIP
}

// peerTrusted reports whether peer may supply forwarding headers: it is a
// configured trusted peer or trusted proxy, or it is private and private
// peers were opted in with [WithTrustPrivatePeers].
func (e *Extractor) peerTrusted(peer netip.Addr) bool {
	return IsTrusted(peer, e.opts.trustedPeers) ||
		IsTrusted(peer, e.opts.trustedProxies) ||
		(e.opts.trustPrivatePeers && IsPrivate(peer))
}

// isForwardedFor reports whether header names X-Forwarded-For. Header names
// are case-insensitive, and gRPC metadata keys are lowercase, so a configured
// "x-forwarded-for" must still get the right-to-left treatment.
func isForwardedFor(header string) bool {
	return strings.EqualFold(header, HeaderXForwardedFor)
}

func (e *Extractor) getIPsFromHeader(ctx context.Context, header string, headers HeaderGetter) []string {
	headerValues := headers.GetHeader(header)
	if len(headerValues) == 0 {
		return nil
	}

	if e.debugEnabled(ctx) {
		e.opts.logger.DebugContext(ctx, "processing header",
			slog.String("header", header), slog.Int("values_count", len(headerValues)))
	}

	if isForwardedFor(header) {
		ips := forwardedForIPs(headerValues)
		if len(ips) > 0 && e.debugEnabled(ctx) {
			e.opts.logger.DebugContext(ctx, "ips from header", slog.String("header", header),
				slog.Any("ips", ips))
		}
		return ips
	}

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
		if e.debugEnabled(ctx) {
			e.opts.logger.DebugContext(ctx, "truncated IPs to limit",
				slog.String("header", header),
				slog.Int("original", originalCount),
				slog.Int("truncated", len(ips)))
		}
	}

	if len(ips) > 0 && e.debugEnabled(ctx) {
		e.opts.logger.DebugContext(ctx, "ips from header", slog.String("header", header),
			slog.Any("ips", ips))
	}

	return ips
}

// forwardedForIPs returns the rightmost [MaxIPsPerHeader] entries of an
// X-Forwarded-For chain in left-to-right order. Each proxy appends to the
// right, so the trusted suffix lives there and the left end is whatever the
// client sent: collecting from the left would let a client pad the chain until
// the proxy-appended entry falls off the limit. Field lines are consumed last
// to first and each one is split from its right end, so only the consumed
// tail is scanned however long the header is. Empty and overlong entries keep
// their positions as empty placeholders so proxy counts cannot shift left.
func forwardedForIPs(values []string) []string {
	ips := make([]string, 0, MaxIPsPerHeader)
	parts := 0
	for _, value := range slices.Backward(values) {
		for parts < MaxIPsPerHeader {
			i := strings.LastIndexByte(value, ',')
			parts++
			if trimmed := strings.TrimSpace(value[i+1:]); trimmed != "" && len(trimmed) <= MaxIPLength {
				ips = append(ips, corestrings.InternString(trimmed))
			} else {
				ips = append(ips, "")
			}
			if i < 0 {
				break
			}
			value = value[:i]
		}
		if parts >= MaxIPsPerHeader {
			break
		}
	}
	slices.Reverse(ips)
	return ips
}

func (e *Extractor) parseAndCheckIP(ctx context.Context, ip, header string) (netip.Addr, bool) {
	// Check cache first if available
	if e.cache != nil {
		if cachedAddr, ok := e.cache.Get(ip); ok {
			if e.debugEnabled(ctx) {
				e.opts.logger.DebugContext(ctx, "found in cache",
					slog.String("ip", cachedAddr.String()),
					slog.Bool("is_private", IsPrivate(cachedAddr)))
			}

			if !IsPrivate(cachedAddr) {
				return cachedAddr, true
			}

			return netip.Addr{}, false
		}
	}

	// Intern the IP string for better memory efficiency
	internedIP := corestrings.InternString(ip)

	addr, err := parseAddr(internedIP)
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

	if e.debugEnabled(ctx) {
		e.opts.logger.DebugContext(ctx, "parsed IP",
			slog.String("ip", addr.String()),
			slog.Bool("is_private", IsPrivate(addr)))
	}

	if !IsPrivate(addr) {
		if e.debugEnabled(ctx) {
			e.opts.logger.DebugContext(ctx, "found public IP",
				slog.String("ip", addr.String()),
				slog.String("header", header))
		}
		return addr, true
	}

	return netip.Addr{}, false
}

// parseAddr parses a header entry and unmaps an IPv4-mapped IPv6 address, so
// "::ffff:10.0.0.1" is classified, cached, and reported as the IPv4 address it
// carries instead of slipping past the IPv4 private and trusted prefixes.
func parseAddr(s string) (netip.Addr, error) {
	addr, err := netip.ParseAddr(s)
	return addr.Unmap(), err
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

	if e.debugEnabled(ctx) {
		e.opts.logger.DebugContext(ctx, "parsing X-Forwarded-For",
			slog.Any("ips", ips),
			slog.Uint64("trusted_proxies_count", uint64(e.opts.trustedProxiesCount)))
	}

	// X-Forwarded-For format: client, proxy1, proxy2, ...
	// We need to find the rightmost IP that is not from a trusted proxy

	// If we have configured trusted proxies count, we can trust that many proxies from the right
	if e.opts.trustedProxiesCount > 0 {
		if uint(len(ips)) <= e.opts.trustedProxiesCount {
			return netip.Addr{}
		}
		index := len(ips) - int(e.opts.trustedProxiesCount) - 1 // #nosec G115 -- proxy count is small
		if index >= 0 && index < len(ips) {
			ip := ips[index]
			if e.debugEnabled(ctx) {
				e.opts.logger.DebugContext(ctx, "checking IP by trusted proxy count",
					slog.String("ip", ip),
					slog.Int("index", index))
			}

			addr, err := parseAddr(ip)
			if err == nil && !IsPrivate(addr) {
				if e.debugEnabled(ctx) {
					e.opts.logger.DebugContext(ctx, "found IP by trusted proxy count",
						slog.String("ip", addr.String()))
				}
				return addr
			}
		}
		return netip.Addr{}
	}

	// Otherwise, iterate from right to left and find the first non-private, non-trusted IP
	for _, ip := range slices.Backward(ips) {
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

		addr, err := parseAddr(internedIP)
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

	// Every entry was private or trusted, so the header names no client this
	// extractor is willing to vouch for. Returning the leftmost one — as this
	// used to — handed back a private address chosen entirely by the caller:
	// the peer must be trusted for the header to be read at all, but its
	// *contents* are whatever the client sent, and a client that can make
	// Extract report 10.0.0.1 walks straight through an ipacl allowlist written
	// for 10.0.0.0/8. Reporting nothing lets Extract apply its documented
	// fallback and answer with the peer address, which is the connection's own
	// rather than a claim.
	return netip.Addr{}
}
