// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"cmp"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/altessa-s/go-slogx/mask"
	"github.com/altessa-s/go-slogx/setup"

	"github.com/altessa-s/go-atlas-core/collections/slices"
	"github.com/altessa-s/go-atlas-core/runtime"
	"github.com/altessa-s/go-atlas-core/runtime/appinfo"

	observabilityconfig "github.com/altessa-s/go-atlas/config/observability"
	corefactory "github.com/altessa-s/go-atlas/factory/core"
	slogx "github.com/altessa-s/go-slogx"
)

// LoggerBuilder assembles a [slog.Logger] from configuration through
// github.com/altessa-s/go-slogx/setup. Create instances with [New].
// The builder is not safe for concurrent use.
type LoggerBuilder struct {
	cfg *observabilityconfig.Logger

	prefixKey     string
	enableMasking bool
	appName       string
	appVersion    string
	serviceID     string

	// built is the logger of the last successful Build; SetLevel and
	// GetLevel act on it.
	built *setup.Logger
}

// New creates a [LoggerBuilder] for the given logger config.
// Config can be nil — the error surfaces at [LoggerBuilder.Build] time.
func New(cfg *observabilityconfig.Logger) *LoggerBuilder {
	return &LoggerBuilder{
		cfg:        cfg,
		prefixKey:  slogx.ModuleKey,
		appName:    appinfo.Name,
		appVersion: appinfo.Version,
	}
}

// Build assembles the logger. It fails on a nil config or any invalid mask
// rule, before building anything. A config at level none yields a discard
// logger. When buffering is enabled, the logger's buffer is drained by a
// [runtime.OnShutdown] hook.
func (b *LoggerBuilder) Build() (*slog.Logger, error) {
	if b.cfg == nil {
		return nil, corefactory.ErrConfigRequired
	}

	maskRules, err := b.resolveMaskRules()
	if err != nil {
		return nil, err
	}

	if b.cfg.Level == observabilityconfig.LoggerLevelNone {
		return slog.New(slog.DiscardHandler), nil
	}

	built, err := setup.New(b.writer(), b.options(maskRules)...)
	if err != nil {
		return nil, err
	}
	b.built = built

	if b.cfg.Buffer.Enabled {
		runtime.OnShutdown(built.Shutdown)
	}

	return built.Logger(), nil
}

// SetLevel changes the default level of the logger built last. Before a
// successful Build it does nothing.
func (b *LoggerBuilder) SetLevel(level slog.Level) {
	if b.built != nil {
		b.built.SetLevel(level)
	}
}

// GetLevel returns the default level of the logger built last, or the
// configured level before a successful Build.
func (b *LoggerBuilder) GetLevel() slog.Level {
	if b.built != nil {
		return b.built.Level()
	}
	if b.cfg == nil {
		return parseLevel("")
	}
	return parseLevel(b.cfg.Level)
}

// writer returns the configured console output.
func (b *LoggerBuilder) writer() io.Writer {
	if b.cfg.Output == observabilityconfig.LoggerConsoleOutputStderr {
		return os.Stderr
	}
	return os.Stdout
}

// options maps the configuration to setup options.
func (b *LoggerBuilder) options(maskRules []mask.Rule) []setup.Option {
	cfg := b.cfg
	opts := []setup.Option{
		setup.WithLevel(parseLevel(cfg.Level)),
		setup.WithSubsystemKey(b.prefixKey),
		setup.WithAddSource(cfg.OutputSource),
		setup.WithTags(cfg.Tags),
		setup.WithAppGroup(cmp.Or(cfg.AppGroupName, setup.DefaultAppGroup)),
		setup.WithAppName(b.appName),
		setup.WithAppVersion(b.appVersion),
		setup.WithServiceID(b.serviceID),
		setup.WithMaskRules(maskRules...),
		setup.WithMaskDefaults(cfg.EnableDefaultMasks || len(cfg.SensitiveTags) > 0 || b.enableMasking),
	}
	opts = slices.AppendIf(opts, cfg.TimeFormat != "", setup.WithTimeFormat(cfg.TimeFormat))
	opts = slices.AppendIf(opts, cfg.MaskString != "", setup.WithMaskString(cfg.MaskString))
	opts = slices.AppendIf(opts, len(cfg.Subsystems) > 0, setup.WithSubsystemLevels(subsystemLevels(cfg.Subsystems)))
	opts = slices.AppendIf(opts, cfg.Buffer.Enabled,
		setup.WithBufferSize(cmp.Or(cfg.Buffer.Size, observabilityconfig.DefaultLoggerBuffer().Size)),
		setup.WithBufferBypassLevel(parseLevel(cfg.Buffer.BypassLevel)))
	opts = slices.AppendIf(opts, !cfg.Colorized, setup.WithColor(setup.ColorNever))

	customHandlersMu.RLock()
	custom, ok := customHandlers[cfg.OutputFormat]
	customHandlersMu.RUnlock()
	switch {
	case ok:
		opts = append(opts, setup.WithSink(func(w io.Writer, o *slog.HandlerOptions) slog.Handler {
			return custom(w, cfg, o)
		}))
	case cfg.OutputFormat == observabilityconfig.LogFormatJSON:
		opts = append(opts, setup.WithFormat(setup.JSON))
	}
	return opts
}

// resolveMaskRules maps the configured mask rules and sensitive tags to
// setup rules, reporting every invalid rule.
func (b *LoggerBuilder) resolveMaskRules() ([]mask.Rule, error) {
	catalog, err := newCatalog()
	if err != nil {
		return nil, err
	}
	rules := make([]mask.Rule, 0, len(b.cfg.MaskRules)+len(b.cfg.SensitiveTags))
	var errs []error
	for i, rule := range b.cfg.MaskRules {
		r, err := maskRule(catalog, rule)
		if err != nil {
			errs = append(errs, fmt.Errorf("invalid mask rule %d (%s): %w",
				i, cmp.Or(rule.Field, rule.Pattern), err))
			continue
		}
		rules = append(rules, r)
	}
	for _, tag := range b.cfg.SensitiveTags {
		rules = append(rules, mask.Rule{Field: tag, Mask: mask.Full()})
	}
	return rules, errors.Join(errs...)
}

// maskRule resolves one configured mask rule against catalog.
func maskRule(catalog *mask.Catalog, rule observabilityconfig.LoggerMaskRule) (mask.Rule, error) {
	if err := rule.Validate(); err != nil {
		return mask.Rule{}, err
	}
	fn, err := catalog.Resolve(rule.Type, rule.Params)
	if err != nil {
		return mask.Rule{}, err
	}
	r := mask.Rule{Field: rule.Field, Mask: fn}
	if rule.Field == "" {
		r.Pattern = rule.Pattern
	}
	return r, r.Validate()
}

// newCatalog returns the go-slogx mask catalog extended with the mask type
// names the go-atlas logger configuration also accepts: smart (first and last
// two runes kept), hash (prefix + "[HASH]"), and the cc and cached_partial
// aliases of credit_card and partial.
func newCatalog() (*mask.Catalog, error) {
	base, err := mask.NewCatalog()
	if err != nil {
		return nil, err
	}
	smart, err := mask.Partial(2, 2, '*') //nolint:mnd // the smart mask keeps two runes on each side
	if err != nil {
		return nil, err
	}
	alias := func(name string) mask.Factory {
		return func(p mask.Params) (mask.Func, error) { return base.Resolve(name, p) }
	}
	return mask.NewCatalog(mask.WithFactories(map[string]mask.Factory{
		"smart": func(mask.Params) (mask.Func, error) { return smart, nil },
		"hash": func(p mask.Params) (mask.Func, error) {
			prefix, err := p.String("prefix", "")
			if err != nil {
				return nil, err
			}
			return mask.Fixed(prefix + "[HASH]"), nil
		},
		"cc":             alias("credit_card"),
		"cached_partial": alias("partial"),
	}))
}

// subsystemLevels parses the configured per-subsystem levels.
func subsystemLevels(levels map[string]observabilityconfig.LoggerLevel) map[string]slog.Level {
	out := make(map[string]slog.Level, len(levels))
	for name, lvl := range levels {
		out[name] = parseLevel(lvl)
	}
	return out
}

// parseLevel maps a configured level to a slog level; unknown values map to
// error.
func parseLevel(level observabilityconfig.LoggerLevel) slog.Level {
	switch level {
	case observabilityconfig.LoggerLevelDebug:
		return slog.LevelDebug
	case observabilityconfig.LoggerLevelInfo:
		return slog.LevelInfo
	case observabilityconfig.LoggerLevelWarning:
		return slog.LevelWarn
	default:
		return slog.LevelError
	}
}
