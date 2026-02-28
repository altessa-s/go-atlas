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
	"slices"
	"strings"

	"github.com/altessa-s/go-atlas/data/cache/lru"
	"github.com/altessa-s/go-atlas/data/limiters"
	"github.com/altessa-s/go-atlas/data/limiters/tokenbucket/storages"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// LimitInfo is an alias for the shared LimitInfo type.
type LimitInfo = limiters.LimitInfo

// ErrNoIPFoundOrInvalid is returned when client IP cannot be determined.
var ErrNoIPFoundOrInvalid = errors.New("no client IP found or invalid IP address")

// ErrLimitExceeded is an alias for the shared ErrLimitExceeded error.
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
	config      *RateLimitConfig
	options     *options
	parsedRules []ParsedRule
	ipCache     lru.Cacher[string, *RateLimitSettings]
	storage     storages.Storage
}

// New creates a new RuleLimiter with the given config and storage.
// Returns an error if config is invalid or the IP cache cannot be created.
//
// Example:
//
//	limiter, err := tokenbucket.New(config, storage)
func New(config *RateLimitConfig, storage storages.Storage, opts ...Option) (*RuleLimiter, error) {
	if err := config.Validate(); err != nil {
		return nil, coreerrs.Wrap(err, "invalid rate limit config")
	}

	options := newOptions(opts...)

	cache, err := lru.NewShardedCache[string, *RateLimitSettings](options.iPCacheSize)
	if err != nil {
		return nil, coreerrs.Wrap(err, "failed to create LRU cache")
	}

	ll := &RuleLimiter{
		config:      config,
		parsedRules: make([]ParsedRule, 0, len(config.Rules)),
		options:     options,
		ipCache:     cache,
		storage:     storage,
	}

	ll.parseAndSortRules()

	return ll, nil
}

func (l *RuleLimiter) parseAndSortRules() {
	l.parsedRules = l.parsedRules[:0]

	// Pre-parse rules
	for _, rule := range l.config.Rules {
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
func (l *RuleLimiter) Limit(ctx context.Context) (*LimitInfo, error) {
	// Check for an authenticated client first
	if token := l.options.extractToken(ctx); token != "" {
		return l.handleRequest(ctx, token)
	}

	// Fall back to IP-based rate limiting
	return l.handleIpBasedRequest(ctx)
}

func (l *RuleLimiter) handleRequest(ctx context.Context, token string) (*LimitInfo, error) {
	if l.options.clientService != nil {
		clientSettings, err := l.options.clientService.Settings(ctx, token)
		if err != nil {
			goto IP // fall back to IP-based limiting on error
		}

		// If no specific settings, fall back to IP-based limiting.
		// IsSkip returns true if clientSettings is nil or indicates skipping.
		if clientSettings.IsSkip() {
			goto IP
		} else if clientSettings.IsUnlimited() {
			return clientSettings.LimitInfo(), nil // No rate limiting for this client
		}

		key := fmt.Sprintf("token-%s", token)
		return l.applyRateLimit(ctx, key, clientSettings)
	}

IP:
	// Fall back to IP-based limiting if no client settings found
	return l.handleIpBasedRequest(ctx)
}

func (l *RuleLimiter) handleIpBasedRequest(ctx context.Context) (*LimitInfo, error) {
	ip := l.options.extractClientIPAddress(ctx)
	if ip == "" {
		if l.options.logger != nil {
			l.options.logger.ErrorContext(ctx, "no client ip address found")
		}
		return nil, ErrNoIPFoundOrInvalid
	}

	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		if l.options.logger != nil {
			l.options.logger.ErrorContext(ctx, "invalid client ip address", "ip", ip)
		}
		return nil, ErrNoIPFoundOrInvalid
	}

	// Find matching rule for this IP
	settings := l.findRateLimitRuleByIp(ctx, parsedIP)
	key := fmt.Sprintf("ip-%s", ip)
	return l.applyRateLimit(ctx, key, settings)
}

func (l *RuleLimiter) findRateLimitRuleByIp(ctx context.Context, ip net.IP) *RateLimitSettings {
	ipStr := ip.String()

	// Check cache first
	if cached, ok := l.ipCache.Get(ipStr); ok {
		if l.options.logger != nil {
			l.options.logger.DebugContext(ctx, "cache hit for IP", "ip", ipStr)
		}
		return cached
	}

	rules := l.parsedRules

	// Iterate through pre-sorted rules
	for _, rule := range rules {
		if rule.ip != nil && rule.ip.Equal(ip) {
			// Exact IP match
			l.ipCache.Put(ipStr, rule.settings)
			if l.options.logger != nil {
				l.options.logger.DebugContext(ctx, "exact IP match found", "ip", ipStr)
			}
			return rule.settings
		}

		if rule.ipNet != nil && rule.ipNet.Contains(ip) {
			// CIDR match
			l.ipCache.Put(ipStr, rule.settings)
			if l.options.logger != nil {
				l.options.logger.DebugContext(ctx, "cidr match found", "ip", ipStr, "cidr", rule.ipNet.String())
			}
			return rule.settings
		}
	}

	if l.options.logger != nil {
		l.options.logger.DebugContext(ctx, "no specific rule found, applying default limit", "ip", ipStr)
	}
	l.ipCache.Put(ipStr, &l.config.Default)
	return &l.config.Default
}

func (l *RuleLimiter) applyRateLimit(ctx context.Context, key string, settings *RateLimitSettings) (*LimitInfo, error) {
	key = strings.ReplaceAll(key, ".", "_")
	providerInfo, err := l.storage.Allow(ctx, key, settings.Limit, settings.Period)

	tl := settings.LimitInfo()
	if providerInfo != nil {
		tl.Reset = providerInfo.Reset
		tl.Remaining = providerInfo.Remaining
	}

	if err != nil {
		if errors.Is(err, storages.ErrLimitExceeded) {
			return tl, ErrLimitExceeded
		}
		if l.options.logger != nil {
			l.options.logger.ErrorContext(ctx, "storage provider returned nil info", "error", err)
		}
	}

	return tl, err
}
