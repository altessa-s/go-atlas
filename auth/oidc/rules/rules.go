// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package rules

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"math"
	"strconv"
	"sync"
	"time"

	"cel.dev/cel-go/cel"
	"cel.dev/cel-go/common/types"
	"cel.dev/cel-go/ext"

	"github.com/altessa-s/go-atlas/cache/lru"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
)

var (
	// ErrInvalidConfig indicates invalid service configuration.
	ErrInvalidConfig = errors.New("invalid service configuration")

	// ErrInvalidCEL indicates an invalid CEL expression.
	ErrInvalidCEL = errors.New("invalid CEL expression")
)

const (
	// DefaultCELCacheSize bounds the shared cache of compiled matchers.
	DefaultCELCacheSize = 1000

	// DefaultCELCostLimit is the maximum runtime cost for a single CEL evaluation.
	// cel-go charges ~1 cost unit per operation; 10000 is generous for claim checks.
	DefaultCELCostLimit uint64 = 10_000

	// DefaultCELMaxExpressionLength is the maximum allowed CEL expression length in bytes.
	DefaultCELMaxExpressionLength = 4096
)

// getCELEnvironment returns the shared CEL environment, initialized on first call.
var getCELEnvironment = sync.OnceValues(func() (*cel.Env, error) {
	return cel.NewEnv(
		cel.Variable("claims", cel.MapType(cel.StringType, cel.DynType)),
		// String helpers (split, join, lowerAscii, …) — required by the
		// generated scopes.any_of rule and available to user rules.
		ext.Strings(),
	)
})

// getCELCache returns the singleton CEL program cache, initialized on first call.
var getCELCache = sync.OnceValue(func() *lru.Cache[string, func(map[string]any) bool] {
	cache, err := lru.NewCache[string, func(map[string]any) bool](DefaultCELCacheSize)
	if err != nil {
		slog.Error("failed to create CEL cache, CEL matchers will recompile on every use",
			"error", err,
			"cache_size", DefaultCELCacheSize)
		return nil
	}
	return cache
})

// CachedCEL returns the compiled matcher shared by provider and service-policy
// preset rules. A cache hit is usable even when ctx has already been canceled.
func CachedCEL(ctx context.Context, expression string) (func(map[string]any) bool, error) {
	cache := getCELCache()
	if cache == nil {
		return compileCEL(expression)
	}

	return cache.GetOrCompute(ctx, expression, func(context.Context) (func(map[string]any) bool, error) {
		return compileCEL(expression)
	})
}

// ValidateCEL checks a validation expression without publishing a preset matcher.
func ValidateCEL(expression string) error {
	_, err := compileCEL(expression)
	return err
}

// CompileCEL synchronously prepares a claims matcher for a preset or validation
// policy, reusing and populating the same cache as CachedCEL.
func CompileCEL(expression string) (func(map[string]any) bool, error) {
	cache := getCELCache()
	if cache == nil {
		return compileCEL(expression)
	}
	if matcher, ok := cache.Get(expression); ok {
		return matcher, nil
	}
	matcher, err := compileCEL(expression)
	if err == nil {
		cache.Put(expression, matcher)
	}
	return matcher, err
}

func compileCEL(expression string) (func(map[string]any) bool, error) {
	if len(expression) > DefaultCELMaxExpressionLength {
		return nil, fmt.Errorf("expression too long (%d bytes, max %d)", len(expression), DefaultCELMaxExpressionLength)
	}

	env, err := getCELEnvironment()
	if err != nil {
		return nil, err
	}

	ast, issues := env.Compile(expression)
	if issues != nil && issues.Err() != nil {
		return nil, coreerrs.WrapOperation(issues.Err(), "compile expression")
	}

	// Check that the expression returns a boolean
	if ast.OutputType() != cel.BoolType {
		return nil, fmt.Errorf("expression must return boolean, got %v", ast.OutputType())
	}

	program, err := env.Program(ast, cel.CostLimit(DefaultCELCostLimit))
	if err != nil {
		return nil, coreerrs.WrapOperation(err, "create program")
	}

	return func(claims map[string]any) bool { return evaluateCELProgram(program, claims) }, nil
}

// maxLeeway mirrors the JWT verifier ceiling. TestCheckLeeway pins the boundary.
const maxLeeway = 5 * time.Minute

// CheckLeeway rejects clock-skew tolerances above the JWT verifier ceiling.
func CheckLeeway(leeway time.Duration) error {
	if leeway > maxLeeway {
		return coreerrs.Wrapf(ErrInvalidConfig, "leeway %s exceeds the maximum %s", leeway, maxLeeway)
	}
	return nil
}

// CheckTokenLifetime rejects a negative minimum or a minimum above an enabled
// maximum. A zero bound disables that limit.
func CheckTokenLifetime(minLifetime, maxLifetime time.Duration) error {
	if minLifetime < 0 {
		return coreerrs.Wrap(ErrInvalidConfig, "minimum token lifetime must be non-negative")
	}
	if maxLifetime > 0 && minLifetime > maxLifetime {
		return coreerrs.Wrap(ErrInvalidConfig, "minimum token lifetime exceeds maximum")
	}
	return nil
}

// evaluateCELProgram evaluates a compiled CEL program with claims, returning false on error.
func evaluateCELProgram(program cel.Program, claims map[string]any) bool {
	// CEL policies historically see JSON numbers as doubles. A number that a
	// double cannot hold exactly is handed over as an int or uint instead, so
	// two distinct identifiers never compare equal. Signed dates stay intact
	// for the other checks and for the returned claims.
	var view map[string]any
	for key, value := range claims {
		if n, ok := value.(json.Number); ok {
			v, ok := celNumber(n)
			if !ok {
				return false
			}
			if view == nil {
				view = maps.Clone(claims)
			}
			view[key] = v
		}
	}
	if view != nil {
		claims = view
	}
	result, _, err := program.Eval(map[string]any{
		"claims": claims,
	})
	if err != nil {
		return false
	}

	boolResult, ok := result.(types.Bool)
	if !ok {
		return false
	}

	return bool(boolResult)
}

// celNumber converts a JSON number for CEL: a double when that holds it
// exactly, an int or uint when it is an integer that a double rounds, and a
// double otherwise.
func celNumber(n json.Number) (any, bool) {
	f, err := n.Float64()
	if err != nil || math.IsInf(f, 0) {
		return nil, false
	}
	if i, err := strconv.ParseInt(string(n), 10, 64); err == nil && !doubleHoldsInt(f, i) {
		return i, true
	}
	if u, err := strconv.ParseUint(string(n), 10, 64); err == nil && !doubleHoldsUint(f, u) {
		return u, true
	}
	return f, true
}

func doubleHoldsInt(f float64, i int64) bool {
	return f >= math.MinInt64 && f < -float64(math.MinInt64) && int64(f) == i
}

func doubleHoldsUint(f float64, u uint64) bool {
	return f >= 0 && f < 1<<64 && uint64(f) == u
}
