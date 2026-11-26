// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package transportconfig

import (
	"errors"
	"fmt"
	"maps"
	"regexp"
	"regexp/syntax"
	"strings"

	"golang.org/x/net/publicsuffix"
)

// ErrCredentialedOriginWildcard indicates an origin pattern is unsafe with credentials.
var ErrCredentialedOriginWildcard = errors.New("cors: insecure configuration: credentialed origin has an unsafe wildcard")

// CompileOriginPattern compiles a configured origin pattern and checks it when credentials are enabled.
func CompileOriginPattern(source string, credentialed bool) (*regexp.Regexp, error) {
	pattern, err := regexp.Compile(source)
	if err != nil {
		return nil, fmt.Errorf("compile origin pattern %q: %w", source, err)
	}
	if credentialed {
		return pattern, ValidateCredentialedOriginPattern(pattern)
	}
	return pattern, nil
}

// ValidateCredentialedOriginPattern requires anchors and confines host wildcards to subdomains of literal registrable domains.
func ValidateCredentialedOriginPattern(pattern *regexp.Regexp) error {
	if pattern == nil {
		return fmt.Errorf("%w: nil origin pattern", ErrCredentialedOriginWildcard)
	}
	re, err := syntax.Parse(pattern.String(), syntax.Perl)
	if err == nil && anchored(re, syntax.OpBeginText) && anchored(re, syntax.OpEndText) &&
		!pattern.MatchString("null") && !pattern.MatchString("https://cors-wildcard-canary.invalid") &&
		!pattern.MatchString("http://cors-wildcard-canary.invalid") &&
		completeOrigins(originPaths(re, originState{})) {
		return nil
	}
	return fmt.Errorf("%w in pattern %q: anchor each alternative, start host wildcards after a literal dot or ://, "+
		"and end the host in a literal registrable domain", ErrCredentialedOriginWildcard, pattern)
}

func completeOrigins(states originStates) bool {
	for state := range states {
		switch state.part {
		case originPort, originIPv6End:
		case originHost:
			if !state.hostComplete() {
				return false
			}
		default:
			return false
		}
	}
	return len(states) > 0
}

func anchored(re *syntax.Regexp, anchor syntax.Op) bool {
	switch re.Op {
	case syntax.OpCapture:
		return anchored(re.Sub[0], anchor)
	case syntax.OpConcat:
		if anchor == syntax.OpBeginText {
			return anchored(re.Sub[0], anchor)
		}
		return anchored(re.Sub[len(re.Sub)-1], anchor)
	case syntax.OpAlternate:
		for _, sub := range re.Sub {
			if !anchored(sub, anchor) {
				return false
			}
		}
		return true
	default:
		return re.Op == anchor
	}
}

type originPart uint8

const (
	originStart originPart = iota
	originScheme
	originColon
	originSlash
	originHost
	originIPv6
	originIPv6End
	originPort
)

type originState struct {
	part        originPart
	hasHost     bool
	boundary    bool
	variable    bool
	literalTail string // Literal text since the last variable host part.
}

func (s originState) hostComplete() bool {
	if !s.hasHost {
		return false
	}
	if !s.variable {
		return true
	}
	// The first literal dot separates the registrable suffix from the variable part.
	_, tail, ok := strings.Cut(s.literalTail, ".")
	if !ok {
		return false
	}
	tail = strings.TrimSuffix(tail, ".")
	tld := tail[strings.LastIndexByte(tail, '.')+1:]
	if !strings.ContainsFunc(tld, asciiLetter) {
		return false // A numeric IPv4 tail is not a registrable suffix.
	}
	// A probe label makes wildcard public suffixes such as *.kawasaki.jp
	// visible even when the literal tail alone is shorter than that rule.
	domain, err := publicsuffix.EffectiveTLDPlusOne("cors-wildcard-canary." + tail)
	return err == nil && len(domain) <= len(tail)
}

func asciiLetter(r rune) bool {
	return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z'
}

func (s originState) literal(r rune) (originState, bool) {
	letter := asciiLetter(r)
	digit := r >= '0' && r <= '9'
	switch s.part {
	case originStart:
		s.part = originScheme
		return s, letter
	case originScheme:
		if r == ':' {
			s.part = originColon
			return s, true
		}
		return s, letter || digit || r == '+' || r == '-' || r == '.'
	case originColon:
		s.part = originSlash
		return s, r == '/'
	case originSlash:
		s.part = originHost
		s.boundary = true
		return s, r == '/'
	case originHost:
		return s.hostLiteral(r)
	case originIPv6:
		if r == ']' {
			s.part = originIPv6End
			return s, true
		}
		return s, letter || digit || r == ':' || r == '.'
	case originIPv6End:
		return originState{part: originPort}, r == ':'
	case originPort:
		return s, digit
	default:
		return s, false
	}
}

func (s originState) hostLiteral(r rune) (originState, bool) {
	switch {
	case r == '[' && !s.hasHost && !s.variable:
		s.part = originIPv6
		return s, true
	case r == ':' && s.hostComplete():
		return originState{part: originPort}, true
	case asciiLetter(r) || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.':
		s.hasHost = true
		s.boundary = r == '.'
		if s.variable {
			s.literalTail += strings.ToLower(string(r))
		}
		return s, true
	}
	return s, false
}

func characterWildcard(op syntax.Op) bool {
	return op == syntax.OpAnyChar || op == syntax.OpAnyCharNotNL || op == syntax.OpCharClass
}

func (s originState) wildcard(re *syntax.Regexp) (originState, bool) {
	switch s.part {
	case originHost:
		if s.boundary {
			return originState{part: originHost, hasHost: true, variable: true}, true
		}
	case originPort:
		if re.Op == syntax.OpCharClass {
			for _, r := range re.Rune {
				if r < '0' || r > '9' {
					return s, false
				}
			}
			return s, true
		}
	default:
		return s, false
	}
	return s, false
}

// Each state describes a possible path through the syntax tree. A nil set
// means a path violated the policy. Empty sets describe impossible matches.
type originStates map[originState]struct{}

// Bound the work for patterns combining many independent alternatives.
const maxOriginStates = 256

func originPaths(re *syntax.Regexp, s originState) originStates {
	switch re.Op {
	case syntax.OpConcat:
		states := originStates{s: {}}
		for _, sub := range re.Sub {
			states = advanceOriginPaths(sub, states)
			if len(states) == 0 {
				return states
			}
		}
		return states
	case syntax.OpAlternate:
		states := make(originStates)
		for _, sub := range re.Sub {
			if !mergeOriginPaths(states, originPaths(sub, s)) {
				return nil
			}
		}
		return states
	case syntax.OpCapture:
		return originPaths(re.Sub[0], s)
	case syntax.OpLiteral:
		for _, r := range re.Rune {
			var ok bool
			s, ok = s.literal(r)
			if !ok {
				return nil
			}
		}
	case syntax.OpAnyChar, syntax.OpAnyCharNotNL, syntax.OpCharClass:
		var ok bool
		s, ok = s.wildcard(re)
		if !ok {
			return nil
		}
	case syntax.OpStar:
		return repeatOriginPaths(re.Sub[0], s, 0, -1)
	case syntax.OpPlus:
		return repeatOriginPaths(re.Sub[0], s, 1, -1)
	case syntax.OpQuest:
		return repeatOriginPaths(re.Sub[0], s, 0, 1)
	case syntax.OpRepeat:
		return repeatOriginPaths(re.Sub[0], s, re.Min, re.Max)
	case syntax.OpNoMatch:
		return make(originStates)
	case syntax.OpEmptyMatch, syntax.OpBeginLine, syntax.OpEndLine,
		syntax.OpBeginText, syntax.OpEndText, syntax.OpWordBoundary, syntax.OpNoWordBoundary:
		// Assertions consume no host text.
	}
	return originStates{s: {}}
}

func advanceOriginPaths(re *syntax.Regexp, states originStates) originStates {
	next := make(originStates)
	for state := range states {
		if !mergeOriginPaths(next, originPaths(re, state)) {
			return nil
		}
	}
	return next
}

func mergeOriginPaths(into, paths originStates) bool {
	if paths == nil {
		return false
	}
	maps.Copy(into, paths)
	return len(into) <= maxOriginStates
}

func repeatOriginPaths(re *syntax.Regexp, s originState, minimum, maximum int) originStates {
	if maximum == 0 {
		return originStates{s: {}}
	}
	for re.Op == syntax.OpCapture {
		re = re.Sub[0]
	}
	// A quantified character wildcard is a single variable host run.
	if characterWildcard(re.Op) {
		states := originPaths(re, s)
		if states != nil && minimum == 0 {
			states[s] = struct{}{}
		}
		return states
	}

	variableWidth := minimum != maximum
	variableHost := variableWidth && s.part == originHost && !startsWithPortSeparator(re)
	if variableHost && !s.boundary {
		return nil
	}

	current := originStates{s: {}}
	result := make(originStates)
	for count := 0; ; count++ {
		if count >= minimum && !mergeOriginPaths(result, current) {
			return nil
		}
		if count == maximum {
			return result
		}

		next := advanceOriginPaths(re, current)
		if next == nil {
			return nil
		}
		if variableHost {
			next = afterVariableHostRepeat(next)
			if next == nil {
				return nil
			}
		}
		if maps.Equal(current, next) || maximum < 0 && count >= minimum && allOriginPathsSeen(next, result) {
			if !mergeOriginPaths(result, next) {
				return nil
			}
			return result
		}
		current = next
	}
}

// Keep the terminating literal dot, if any. Host text contributed by a
// variable-width repeat cannot establish a literal registrable suffix.
func afterVariableHostRepeat(paths originStates) originStates {
	next := make(originStates)
	for state := range paths {
		if state.part != originHost {
			return nil
		}
		state.variable = true
		state.literalTail = ""
		if state.boundary {
			state.literalTail = "."
		}
		next[state] = struct{}{}
	}
	return next
}

func allOriginPathsSeen(states, seen originStates) bool {
	for state := range states {
		if _, ok := seen[state]; !ok {
			return false
		}
	}
	return true
}

// A colon at the start of every alternative marks a port-only repeat.
func startsWithPortSeparator(re *syntax.Regexp) bool {
	switch re.Op {
	case syntax.OpCapture, syntax.OpConcat:
		return startsWithPortSeparator(re.Sub[0])
	case syntax.OpAlternate:
		for _, sub := range re.Sub {
			if !startsWithPortSeparator(sub) {
				return false
			}
		}
		return true
	default:
		return re.Op == syntax.OpLiteral && re.Rune[0] == ':'
	}
}
