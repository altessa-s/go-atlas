// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package tokenbucket

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"slices"
	"strings"

	"github.com/altessa-s/go-atlas/cache/lru"
	"github.com/altessa-s/go-atlas/coord/limiters"
	"github.com/altessa-s/go-atlas/coord/limiters/storages"

	corehash "github.com/altessa-s/go-atlas-core/encoding/hash"
	coreerrs "github.com/altessa-s/go-atlas-core/errors"
)

// ErrNoIPFoundOrInvalid is returned when client IP cannot be determined.
var ErrNoIPFoundOrInvalid = errors.New("no client IP found or invalid IP address")

// ErrLimitExceeded is returned by [RuleLimiter.Limit] when a request exceeds
// its rate limit. It shares its identity with [limiters.ErrLimitExceeded].
var ErrLimitExceeded = limiters.ErrLimitExceeded

// ClientService retrieves rate limit settings for authenticated clients.
// Implementations must be safe for concurrent use.
type ClientService interface {
	// Settings returns rate limits for a token. Return RateLimitSkip for IP fallback.
	Settings(ctx context.Context, token string) (*RateLimitSettings, error)
}

// ClientServiceFunc is a function adapter for ClientService.
type ClientServiceFunc func(ctx context.Context, token string) (*RateLimitSettings, error)

// Settings calls the underlying function.
func (l ClientServiceFunc) Settings(ctx context.Context, token string) (*RateLimitSettings, error) {
	return l(ctx, token)
}

// ParsedRule holds a pre-parsed IP or CIDR for efficient matching.
type ParsedRule struct {
	settings *RateLimitSettings
	ipNet    *net.IPNet
	ip       net.IP
}

// RuleLimiter provides rule-based rate limiting with IP/CIDR and token support.
// It is safe for concurrent use and uses LRU caching for IP lookups.
type RuleLimiter struct {
	defaults    RateLimitSettings
	options     *options
	parsedRules []ParsedRule
	ipCache     lru.Cacher[string, *RateLimitSettings]
	storage     storages.Storage
	metrics     *limiterMetrics
}

// New creates a new RuleLimiter that applies defaults to every client no
// rule in [WithRules] matches, counting requests in storage.
// Returns an error if defaults or a rule is invalid, or the IP cache cannot be
// created.
//
// Example:
//
//	limiter, err := tokenbucket.New(
//		tokenbucket.RateLimitSettings{Limit: 100, Period: time.Minute},
//		storage,
//		tokenbucket.WithRules(&tokenbucket.RateLimitRule{Target: "10.0.0.0/8", RateLimitSettings: internal}),
//	)
func New(defaults RateLimitSettings, storage storages.Storage, opts ...Option) (*RuleLimiter, error) {
	if err := defaults.Validate(); err != nil {
		return nil, coreerrs.Wrap(err, "invalid default settings")
	}

	options := newOptions(opts...)
	if err := validateRules(options.rules); err != nil {
		return nil, err
	}
	if options.iPv6KeyPrefixBits < 0 || options.iPv6KeyPrefixBits > ipv6Bits {
		return nil, fmt.Errorf("IPv6 key prefix bits %d out of range [0, %d]", options.iPv6KeyPrefixBits, ipv6Bits)
	}

	cache, err := lru.NewShardedCache[string, *RateLimitSettings](options.iPCacheSize)
	if err != nil {
		return nil, coreerrs.Wrap(err, "failed to create LRU cache")
	}

	ll := &RuleLimiter{
		defaults:    defaults,
		parsedRules: make([]ParsedRule, 0, len(options.rules)),
		options:     options,
		ipCache:     cache,
		storage:     storage,
		metrics:     newLimiterMetrics(options.meter),
	}

	ll.parseAndSortRules()

	return ll, nil
}

func (l *RuleLimiter) parseAndSortRules() {
	l.parsedRules = l.parsedRules[:0]

	// Pre-parse rules
	for _, rule := range l.options.rules {
		parsed := ParsedRule{settings: rule.RateLimitSettings}
		if ip := net.ParseIP(rule.Target); ip != nil {
			parsed.ip = ip
		} else {
			if _, ipNet, err := net.ParseCIDR(rule.Target); err == nil {
				parsed.ipNet = ipNet
			}
		}
		l.parsedRules = append(l.parsedRules, parsed)
	}

	// Sort rules for efficient matching:
	// 1. Exact IP matches first
	// 2. Most specific CIDR matches (smaller subnets first)
	slices.SortFunc(l.parsedRules, func(a, b ParsedRule) int {
		// IP rules before CIDR rules
		if a.ip != nil && b.ipNet != nil {
			return -1
		}
		if a.ipNet != nil && b.ip != nil {
			return 1
		}

		// For CIDR rules, smaller subnets (more specific) come first
		if a.ipNet != nil && b.ipNet != nil {
			onesA, _ := a.ipNet.Mask.Size()
			onesB, _ := b.ipNet.Mask.Size()
			return cmp.Compare(onesB, onesA) // Reversed: more ones = smaller subnet = first
		}

		// Both are IPs or order doesn't matter
		return 0
	})
}

// Limit applies rate limiting based on token or client IP.
// Returns ErrLimitExceeded if the limit is exceeded.
//
// Example:
//
//	info, err := limiter.Limit(ctx)
func (l *RuleLimiter) Limit(ctx context.Context) (*limiters.LimitInfo, error) {
	var settings *RateLimitSettings
	token := l.options.extractToken(ctx)
	if token != "" && l.options.clientService != nil {
		if clientSettings, err := l.options.clientService.Settings(ctx, token); err == nil {
			settings = clientSettings
		}
	}

	var ip string
	if settings.IsSkip() {
		ip = l.options.extractClientIPAddress(ctx)
		if ip == "" {
			if l.options.logger != nil {
				l.options.logger.ErrorContext(ctx, "no client ip address found")
			}
			return nil, ErrNoIPFoundOrInvalid
		}
		var err error
		settings, err = l.findRateLimitRuleByIp(ctx, ip)
		if err != nil {
			return nil, err
		}
	}

	if settings.IsUnlimited() {
		return settings.LimitInfo(), nil
	}
	if err := settings.Validate(); err != nil {
		l.metrics.limitCheckErrors.Inc()
		return nil, err
	}

	if ip != "" {
		return l.applyRateLimit(ctx, l.storageKeyForAddr(ip), settings)
	}
	// Key by digest so storage backends never hold a bearer token.
	return l.applyRateLimit(ctx, "token-"+corehash.SHA256HexString(token), settings)
}

// ipv6Bits is the length of an IPv6 address in bits.
const ipv6Bits = 128

// storageKeyForAddr builds the storage key for a validated client address.
// IPv4 keys per address. IPv6 keys per configured prefix, so a client
// rotating through the addresses of its own subnet stays in one bucket; an
// IPv4-mapped IPv6 address keys as the IPv4 address it carries.
func (l *RuleLimiter) storageKeyForAddr(ip string) string {
	if strings.IndexByte(ip, ':') < 0 {
		return storageKeyForIP(ip)
	}
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return storageKeyForIP(ip)
	}
	addr = addr.Unmap().WithZone("")
	if addr.Is4() {
		return storageKeyForIP(addr.String())
	}
	return "ip-" + netip.PrefixFrom(addr, l.options.iPv6KeyPrefixBits).Masked().String()
}

// storageKeyForIP builds "ip-<addr>" with dots translated to underscores in
// a single pass, matching the sanitization applyRateLimit performs.
func storageKeyForIP(ip string) string {
	b := make([]byte, 0, len(ip)+3)
	b = append(b, "ip-"...)
	for i := range len(ip) {
		c := ip[i]
		if c == '.' {
			c = '_'
		}
		b = append(b, c)
	}
	return string(b)
}

// findRateLimitRuleByIp resolves the rule for the client IP. The LRU is
// keyed by the caller's original string, so the hot path is a single cache
// lookup with no parse or re-serialization; parsing happens only on a miss.
func (l *RuleLimiter) findRateLimitRuleByIp(ctx context.Context, ipStr string) (*RateLimitSettings, error) {
	// Check cache first
	if cached, ok := l.ipCache.Get(ipStr); ok {
		if l.options.logger != nil {
			l.options.logger.DebugContext(ctx, "cache hit for IP", "ip", ipStr)
		}
		return cached, nil
	}

	addr, err := netip.ParseAddr(ipStr)
	if err != nil {
		if l.options.logger != nil {
			l.options.logger.ErrorContext(ctx, "invalid client ip address", "ip", ipStr)
		}
		return nil, ErrNoIPFoundOrInvalid
	}
	ip := net.IP(addr.AsSlice())

	rules := l.parsedRules

	// Iterate through pre-sorted rules
	for _, rule := range rules {
		if rule.ip != nil && rule.ip.Equal(ip) {
			// Exact IP match
			l.ipCache.Put(ipStr, rule.settings)
			if l.options.logger != nil {
				l.options.logger.DebugContext(ctx, "exact IP match found", "ip", ipStr)
			}
			return rule.settings, nil
		}

		if rule.ipNet != nil && rule.ipNet.Contains(ip) {
			// CIDR match
			l.ipCache.Put(ipStr, rule.settings)
			if l.options.logger != nil {
				l.options.logger.DebugContext(ctx, "cidr match found", "ip", ipStr, "cidr", rule.ipNet.String())
			}
			return rule.settings, nil
		}
	}

	if l.options.logger != nil {
		l.options.logger.DebugContext(ctx, "no specific rule found, applying default limit", "ip", ipStr)
	}
	l.ipCache.Put(ipStr, &l.defaults)
	return &l.defaults, nil
}

func (l *RuleLimiter) applyRateLimit(ctx context.Context, key string, settings *RateLimitSettings) (*limiters.LimitInfo, error) {
	key = strings.ReplaceAll(key, ".", "_")
	providerInfo, err := l.storage.Allow(ctx, key, settings.Limit, settings.Period)

	tl := settings.LimitInfo()
	if providerInfo != nil {
		tl.Reset = providerInfo.Reset
		tl.Remaining = providerInfo.Remaining
	}

	if err != nil {
		// limiters.ErrLimitExceeded is the public sentinel by identity, so the
		// storage error already satisfies errors.Is(err, ErrLimitExceeded).
		if errors.Is(err, limiters.ErrLimitExceeded) {
			l.metrics.requestsRejected.Inc()
			return tl, err
		}
		l.metrics.limitCheckErrors.Inc()
		if l.options.logger != nil {
			l.options.logger.ErrorContext(ctx, "storage provider returned nil info", "error", err)
		}
		return tl, err
	}

	l.metrics.requestsAllowed.Inc()
	return tl, nil
}
