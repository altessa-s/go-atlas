// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package ipacl

import (
	"net/netip"
	"regexp"
	"testing"
)

func prefix(s string) netip.Prefix { return netip.MustParsePrefix(s) }
func addr(s string) netip.Addr     { return netip.MustParseAddr(s) }

func TestEvaluate_AllowlistMode(t *testing.T) {
	reg := NewRegistry(PolicyDeny)
	reg.Register("/admin.AdminService/Delete", &AccessRule{
		Allowlist: []netip.Prefix{prefix("10.1.1.0/24")},
	})

	tests := []struct {
		name     string
		ip       string
		endpoint string
		want     bool
	}{
		{"allowed IP", "10.1.1.5", "/admin.AdminService/Delete", true},
		{"denied IP", "192.168.1.1", "/admin.AdminService/Delete", false},
		{"no rule, policy deny", "10.1.1.5", "/other.Service/Method", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := reg.Evaluate(addr(tt.ip), tt.endpoint); got != tt.want {
				t.Errorf("Evaluate(%s, %s) = %v, want %v", tt.ip, tt.endpoint, got, tt.want)
			}
		})
	}
}

func TestEvaluate_DenylistMode(t *testing.T) {
	reg := NewRegistry(PolicyAllow)
	reg.Register("/api.Service/Action", &AccessRule{
		Denylist: []netip.Prefix{prefix("10.99.0.0/16")},
	})

	tests := []struct {
		name     string
		ip       string
		endpoint string
		want     bool
	}{
		{"denied IP", "10.99.1.1", "/api.Service/Action", false},
		{"allowed IP", "192.168.1.1", "/api.Service/Action", true},
		{"no rule, policy allow", "10.99.1.1", "/other.Service/Method", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := reg.Evaluate(addr(tt.ip), tt.endpoint); got != tt.want {
				t.Errorf("Evaluate(%s, %s) = %v, want %v", tt.ip, tt.endpoint, got, tt.want)
			}
		})
	}
}

func TestEvaluate_DenyWinsOnOverlap(t *testing.T) {
	reg := NewRegistry(PolicyDeny)
	reg.Register("/api.Service/Action", &AccessRule{
		Allowlist: []netip.Prefix{prefix("10.0.0.0/8")},
		Denylist:  []netip.Prefix{prefix("10.99.0.0/16")},
	})

	tests := []struct {
		name string
		ip   string
		want bool
	}{
		{"in both lists, deny wins", "10.99.1.1", false},
		{"only in allowlist", "10.1.1.1", true},
		{"in neither", "192.168.1.1", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := reg.Evaluate(addr(tt.ip), "/api.Service/Action"); got != tt.want {
				t.Errorf("Evaluate(%s) = %v, want %v", tt.ip, got, tt.want)
			}
		})
	}
}

func TestEvaluate_PatternMatch(t *testing.T) {
	reg := NewRegistry(PolicyDeny)
	reg.RegisterPattern(regexp.MustCompile(`^/internal\..*`), &AccessRule{
		Allowlist: []netip.Prefix{prefix("10.0.0.0/8")},
	})

	tests := []struct {
		name     string
		ip       string
		endpoint string
		want     bool
	}{
		{"pattern match, allowed", "10.1.1.1", "/internal.Svc/Do", true},
		{"pattern match, denied", "192.168.1.1", "/internal.Svc/Do", false},
		{"no pattern match", "10.1.1.1", "/public.Svc/Do", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := reg.Evaluate(addr(tt.ip), tt.endpoint); got != tt.want {
				t.Errorf("Evaluate(%s, %s) = %v, want %v", tt.ip, tt.endpoint, got, tt.want)
			}
		})
	}
}

func TestEvaluate_DefaultRule(t *testing.T) {
	reg := NewRegistry(PolicyDeny)
	reg.SetDefault(&AccessRule{
		Allowlist: []netip.Prefix{prefix("10.0.0.0/8"), prefix("192.168.0.0/16")},
	})

	tests := []struct {
		name string
		ip   string
		want bool
	}{
		{"default allows private", "10.1.1.1", true},
		{"default allows 192.168", "192.168.1.1", true},
		{"default denies public", "8.8.8.8", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := reg.Evaluate(addr(tt.ip), "/any.Endpoint"); got != tt.want {
				t.Errorf("Evaluate(%s) = %v, want %v", tt.ip, got, tt.want)
			}
		})
	}
}

func TestEvaluate_IPv6(t *testing.T) {
	reg := NewRegistry(PolicyDeny)
	reg.Register("/api.Service/Action", &AccessRule{
		Allowlist: []netip.Prefix{prefix("2001:db8::/32")},
		Denylist:  []netip.Prefix{prefix("2001:db8:dead::/48")},
	})

	tests := []struct {
		name string
		ip   string
		want bool
	}{
		{"allowed IPv6", "2001:db8::1", true},
		{"denied IPv6 subnet", "2001:db8:dead::1", false},
		{"not in list IPv6", "2001:4860::1", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := reg.Evaluate(addr(tt.ip), "/api.Service/Action"); got != tt.want {
				t.Errorf("Evaluate(%s) = %v, want %v", tt.ip, got, tt.want)
			}
		})
	}
}

func TestEvaluate_IPv4MappedIPv6(t *testing.T) {
	reg := NewRegistry(PolicyDeny)
	reg.Register("/api.Service/Action", &AccessRule{
		Allowlist: []netip.Prefix{prefix("10.0.0.0/8")},
		Denylist:  []netip.Prefix{prefix("10.99.0.0/16")},
	})

	tests := []struct {
		name string
		ip   string
		want bool
	}{
		{"IPv4-mapped allowed", "::ffff:10.1.1.1", true},
		{"IPv4-mapped denied", "::ffff:10.99.1.1", false},
		{"IPv4-mapped not in list", "::ffff:192.168.1.1", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := reg.Evaluate(addr(tt.ip), "/api.Service/Action"); got != tt.want {
				t.Errorf("Evaluate(%s) = %v, want %v", tt.ip, got, tt.want)
			}
		})
	}
}

func TestEvaluate_NoRule_PolicyApplies(t *testing.T) {
	t.Run("PolicyDeny", func(t *testing.T) {
		reg := NewRegistry(PolicyDeny)
		if reg.Evaluate(addr("1.2.3.4"), "/some.Endpoint") {
			t.Error("expected deny when no rule and PolicyDeny")
		}
	})

	t.Run("PolicyAllow", func(t *testing.T) {
		reg := NewRegistry(PolicyAllow)
		if !reg.Evaluate(addr("1.2.3.4"), "/some.Endpoint") {
			t.Error("expected allow when no rule and PolicyAllow")
		}
	})
}

func TestEvaluate_EmptyLists_FallsBackToPolicy(t *testing.T) {
	t.Run("PolicyDeny", func(t *testing.T) {
		reg := NewRegistry(PolicyDeny)
		reg.Register("/api.Service/Action", &AccessRule{})
		if reg.Evaluate(addr("10.1.1.1"), "/api.Service/Action") {
			t.Error("expected deny when rule has empty lists and PolicyDeny")
		}
	})

	t.Run("PolicyAllow", func(t *testing.T) {
		reg := NewRegistry(PolicyAllow)
		reg.Register("/api.Service/Action", &AccessRule{})
		if !reg.Evaluate(addr("10.1.1.1"), "/api.Service/Action") {
			t.Error("expected allow when rule has empty lists and PolicyAllow")
		}
	})
}

func TestLookup_ExactBeforePattern(t *testing.T) {
	reg := NewRegistry(PolicyDeny)

	exactRule := &AccessRule{Allowlist: []netip.Prefix{prefix("10.1.1.0/24")}}
	patternRule := &AccessRule{Allowlist: []netip.Prefix{prefix("10.0.0.0/8")}}

	reg.Register("/svc.Service/Method", exactRule)
	reg.RegisterPattern(regexp.MustCompile(`^/svc\..*`), patternRule)

	got, ok := reg.Lookup("/svc.Service/Method")
	if !ok {
		t.Fatal("expected to find a rule")
	}
	if got != exactRule {
		t.Error("exact match should take priority over pattern match")
	}
}

func TestRegisterEndpoints(t *testing.T) {
	reg := NewRegistry(PolicyDeny)
	rule := &AccessRule{Allowlist: []netip.Prefix{prefix("10.0.0.0/8")}}
	reg.RegisterEndpoints(rule, "/a.Svc/A", "/b.Svc/B")

	for _, ep := range []string{"/a.Svc/A", "/b.Svc/B"} {
		got, ok := reg.Lookup(ep)
		if !ok || got != rule {
			t.Errorf("expected rule for endpoint %s", ep)
		}
	}
}
