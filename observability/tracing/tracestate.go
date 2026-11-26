// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package tracing

import (
	"slices"
	"strings"
)

// W3C Trace Context tracestate limits.
const (
	MaxTraceStateMembers       = 32
	MaxTraceStateLen           = 512
	maxTraceStateOversizedItem = 128
	maxTraceStateRawLen        = 4096
	maxTraceStateKeyLen        = 256
	maxTraceStateValLen        = 256
	maxTenantIDLen             = 241
	maxSystemIDLen             = 14
)

// ParseTraceState validates raw against the W3C Trace Context tracestate
// grammar, normalizes optional whitespace and empty list elements, and bounds
// the result to at most [MaxTraceStateMembers] entries and [MaxTraceStateLen]
// bytes. When valid input exceeds the byte budget, entries longer than 128
// bytes are removed from right to left first, followed by trailing entries
// until both limits are satisfied. Any syntactically invalid entry or
// duplicate key rejects the entire header.
func ParseTraceState(raw string) (string, bool) {
	if raw == "" || len(raw) > maxTraceStateRawLen {
		return "", false
	}

	var (
		seen       traceStateKeySet
		list       traceStateList
		needsBuild bool
	)

	for part := range strings.SplitSeq(raw, ",") {
		member := trimOWS(part)
		if member == "" {
			needsBuild = true
			continue
		}
		if len(member) != len(part) {
			needsBuild = true
		}

		key, val, ok := strings.Cut(member, "=")
		if !ok || !isValidTraceStateKey(key) || !isValidTraceStateValue(val) || !seen.add(key) {
			return "", false
		}
		list.append(member)
	}

	if list.count == 0 {
		return "", false
	}
	if list.count <= MaxTraceStateMembers && list.byteLen <= MaxTraceStateLen {
		if !needsBuild && list.byteLen == len(raw) {
			return raw, true
		}
		return joinTraceStateMembers(list.arr[:list.count], list.byteLen), true
	}

	items := list.materialize()
	byteLen := list.byteLen
	if byteLen > MaxTraceStateLen {
		for i := len(items) - 1; i >= 0 && byteLen > MaxTraceStateLen; i-- {
			if len(items[i]) > maxTraceStateOversizedItem {
				byteLen -= removedMemberBytes(len(items[i]), len(items))
				items = slices.Delete(items, i, i+1)
			}
		}
	}
	for len(items) > 0 && (len(items) > MaxTraceStateMembers || byteLen > MaxTraceStateLen) {
		last := items[len(items)-1]
		byteLen -= removedMemberBytes(len(last), len(items))
		items = items[:len(items)-1]
	}
	if len(items) == 0 {
		return "", false
	}
	return joinTraceStateMembers(items, byteLen), true
}

type traceStateKeySet struct {
	arr  [MaxTraceStateMembers]string
	more map[string]struct{}
	n    int
}

func (s *traceStateKeySet) add(key string) bool {
	if slices.Contains(s.arr[:min(s.n, MaxTraceStateMembers)], key) {
		return false
	}
	if s.more != nil {
		if _, exists := s.more[key]; exists {
			return false
		}
	}
	if s.n < MaxTraceStateMembers {
		s.arr[s.n] = key
	} else {
		if s.more == nil {
			s.more = make(map[string]struct{}, 8)
		}
		s.more[key] = struct{}{}
	}
	s.n++
	return true
}

type traceStateList struct {
	arr     [MaxTraceStateMembers]string
	more    []string
	count   int
	byteLen int
}

func (l *traceStateList) append(member string) {
	if l.count > 0 {
		l.byteLen++
	}
	l.byteLen += len(member)
	if l.count < MaxTraceStateMembers {
		l.arr[l.count] = member
	} else {
		l.more = append(l.more, member)
	}
	l.count++
}

func (l *traceStateList) materialize() []string {
	items := make([]string, l.count)
	n := copy(items, l.arr[:min(l.count, MaxTraceStateMembers)])
	copy(items[n:], l.more)
	return items
}

func removedMemberBytes(memberLen, currentCount int) int {
	if currentCount <= 1 {
		return memberLen
	}
	return memberLen + 1
}

func joinTraceStateMembers(items []string, byteLen int) string {
	var b strings.Builder
	b.Grow(byteLen)
	for i, item := range items {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(item)
	}
	return b.String()
}

func trimOWS(s string) string {
	start := 0
	for start < len(s) && (s[start] == ' ' || s[start] == '\t') {
		start++
	}
	end := len(s)
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t') {
		end--
	}
	return s[start:end]
}

func isValidTraceStateKey(key string) bool {
	if len(key) == 0 || len(key) > maxTraceStateKeyLen {
		return false
	}
	tenant, system, hasAt := strings.Cut(key, "@")
	if !hasAt {
		return isLowerAlpha(key[0]) && isKeyTail(key[1:])
	}
	if len(tenant) == 0 || len(tenant) > maxTenantIDLen || len(system) == 0 || len(system) > maxSystemIDLen {
		return false
	}
	if !isLowerAlphaOrDigit(tenant[0]) || !isKeyTail(tenant[1:]) {
		return false
	}
	return isLowerAlpha(system[0]) && isKeyTail(system[1:])
}

func isLowerAlpha(c byte) bool {
	return c >= 'a' && c <= 'z'
}

func isLowerAlphaOrDigit(c byte) bool {
	return isLowerAlpha(c) || (c >= '0' && c <= '9')
}

func isKeyTail(s string) bool {
	for i := range len(s) {
		c := s[i]
		if !isLowerAlphaOrDigit(c) && c != '_' && c != '-' && c != '*' && c != '/' {
			return false
		}
	}
	return true
}

func isValidTraceStateValue(val string) bool {
	if len(val) == 0 || len(val) > maxTraceStateValLen {
		return false
	}
	// Last character must be nblk-chr (printable ASCII excluding space, comma, equals).
	last := val[len(val)-1]
	if last < 0x21 || last > 0x7e || last == ',' || last == '=' {
		return false
	}
	for i := range len(val) - 1 {
		c := val[i]
		if c < 0x20 || c > 0x7e || c == ',' || c == '=' {
			return false
		}
	}
	return true
}
