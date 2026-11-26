// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package cors

import (
	"regexp"
	"regexp/syntax"
)

type originSuffix uint8

const (
	otherSuffix originSuffix = iota
	colonSuffix
	colonSlashSuffix
	hostBoundary // A literal dot or ://.
	unsafeWildcard
)

func hostWildcardsBounded(pattern *regexp.Regexp) bool {
	re, err := syntax.Parse(pattern.String(), syntax.Perl)
	return err == nil && checkOriginPattern(re, otherSuffix) != unsafeWildcard
}

// checkOriginPattern tracks the suffix guaranteed on every path through re.
// Checking the syntax tree distinguishes wildcard dots from literal dots and
// prevents an optional separator or just one alternative from permitting a wildcard.
func checkOriginPattern(re *syntax.Regexp, suffix originSuffix) originSuffix {
	switch re.Op {
	case syntax.OpConcat:
		for _, sub := range re.Sub {
			suffix = checkOriginPattern(sub, suffix)
			if suffix == unsafeWildcard {
				return suffix
			}
		}
		return suffix
	case syntax.OpAlternate:
		result := checkOriginPattern(re.Sub[0], suffix)
		for _, sub := range re.Sub[1:] {
			result = mergeOriginSuffix(result, checkOriginPattern(sub, suffix))
		}
		return result
	case syntax.OpCapture:
		return checkOriginPattern(re.Sub[0], suffix)
	case syntax.OpLiteral:
		for _, r := range re.Rune {
			switch {
			case r == '.' || r == '/' && suffix == colonSlashSuffix:
				suffix = hostBoundary
			case r == ':':
				suffix = colonSuffix
			case r == '/' && suffix == colonSuffix:
				suffix = colonSlashSuffix
			default:
				suffix = otherSuffix
			}
		}
		return suffix
	case syntax.OpAnyChar, syntax.OpAnyCharNotNL:
		if suffix != hostBoundary {
			return unsafeWildcard
		}
		return otherSuffix
	case syntax.OpCharClass:
		return otherSuffix
	case syntax.OpStar:
		return checkOriginRepeat(re.Sub[0], suffix, 0, -1)
	case syntax.OpPlus:
		return checkOriginRepeat(re.Sub[0], suffix, 1, -1)
	case syntax.OpQuest:
		return checkOriginRepeat(re.Sub[0], suffix, 0, 1)
	case syntax.OpRepeat:
		return checkOriginRepeat(re.Sub[0], suffix, re.Min, re.Max)
	default:
		return suffix // Anchors and other zero-width assertions consume no text.
	}
}

func mergeOriginSuffix(a, b originSuffix) originSuffix {
	switch {
	case a == unsafeWildcard || b == unsafeWildcard:
		return unsafeWildcard
	case a == b:
		return a
	default:
		return otherSuffix
	}
}

func checkOriginRepeat(re *syntax.Regexp, suffix originSuffix, minCount, maxCount int) originSuffix {
	if maxCount == 0 {
		return suffix
	}
	for re.Op == syntax.OpCapture {
		re = re.Sub[0]
	}
	switch re.Op {
	case syntax.OpAnyChar, syntax.OpAnyCharNotNL, syntax.OpCharClass:
		// A quantified character wildcard is one run. A decimal run immediately
		// after a colon is a port and does not match any part of the hostname.
		decimalPort := suffix == colonSuffix && re.Op == syntax.OpCharClass &&
			len(re.Rune) == 2 && re.Rune[0] >= '0' && re.Rune[1] <= '9'
		if suffix != hostBoundary && !decimalPort {
			return unsafeWildcard
		}
		return otherSuffix
	default:
	}

	for range minCount {
		next := checkOriginPattern(re, suffix)
		if next == unsafeWildcard {
			return next
		}
		if next == suffix {
			break
		}
		suffix = next
	}
	for count := minCount; maxCount < 0 || count < maxCount; count++ {
		next := mergeOriginSuffix(suffix, checkOriginPattern(re, suffix))
		if next == suffix || next == unsafeWildcard {
			return next
		}
		suffix = next
	}
	return suffix
}
