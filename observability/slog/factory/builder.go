// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"cmp"
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/mattn/go-isatty"

	"github.com/altessa-s/go-atlas/config"
	"github.com/altessa-s/go-atlas/core/collections/maps"
	"github.com/altessa-s/go-atlas/core/collections/slices"
	"github.com/altessa-s/go-atlas/core/runtime"
	"github.com/altessa-s/go-atlas/core/runtime/appinfo"
	"github.com/altessa-s/go-atlas/observability/slog/handler/buffered"
	"github.com/altessa-s/go-atlas/observability/slog/handler/colorized"
	"github.com/altessa-s/go-atlas/observability/slog/handler/leveled"
	"github.com/altessa-s/go-atlas/observability/slog/handler/masking"
	"github.com/altessa-s/go-atlas/observability/slog/handler/prefixed"

	corefactory "github.com/altessa-s/go-atlas/core/factory"
	slogx "github.com/altessa-s/go-atlas/observability/slog"
)

// LoggerBuilder assembles a [slog.Logger] from configuration using a fluent API
// with deferred error accumulation.
//
// After [LoggerBuilder.Build], the builder retains a reference to the [slog.LevelVar]
// and can be used for runtime level changes via [LoggerBuilder.SetLevel] and
// [LoggerBuilder.GetLevel].
type LoggerBuilder struct {
	cfg  *config.Logger
	errs []error

	// Configuration (set via With*).
	prefixKey     string
	prefixColors  map[string][]int
	enableMasking bool
	appName       string
	appVersion    string
	serviceId     string
	levelVar      *slog.LevelVar
}

// New creates a new [LoggerBuilder] for the given logger config.
func New(cfg *config.Logger) *LoggerBuilder {
	return &LoggerBuilder{
		cfg:          cfg,
		prefixKey:    ModuleKey,
		prefixColors: map[string][]int{ModuleKey: {46}},
		appName:      appinfo.Name,
		appVersion:   appinfo.Version,
	}
}

// Build assembles and returns the logger.
func (b *LoggerBuilder) Build() (*slog.Logger, error) {
	if err := corefactory.JoinErrors(b.errs); err != nil {
		return nil, err
	}

	if b.cfg == nil {
		return nil, fmt.Errorf("configuration is required")
	}
	if b.cfg.Level == config.LoggerLevelNone {
		return slog.New(slog.DiscardHandler), nil
	}

	maskString := cmp.Or(b.cfg.MaskString, "****")
	appGroupName := cmp.Or(b.cfg.AppGroupName, "app")

	writer := b.getWriter()
	handler := b.createHandler(writer, maskString)

	handler = b.wrapWithPrefixedHandler(handler)
	handler = b.wrapWithLeveledHandler(handler)

	// Enable the advanced masking wrapper when:
	// - EnableDefaultMasks is true (explicit opt-in for standard masks)
	// - MaskRules are configured (custom mask rules)
	// - SensitiveTags are declared (backward compatibility)
	// - EnableMasking is set programmatically
	if b.cfg.EnableDefaultMasks || len(b.cfg.MaskRules) > 0 ||
		len(b.cfg.SensitiveTags) > 0 || b.enableMasking {
		handler = b.wrapWithMaskingHandler(handler, maskString)
	}

	if b.cfg.Buffer.Enabled {
		handler = buffered.NewHandler(handler,
			buffered.WithBufferSize(b.cfg.Buffer.Size),
			buffered.WithBypassLevel(b.parseLevel(b.cfg.Buffer.BypassLevel)),
		)
	}

	logger := slog.New(handler)

	if b.cfg.Buffer.Enabled {
		runtime.OnShutdown(func(ctx context.Context) error {
			return slogx.Shutdown(ctx, logger)
		})
	}

	logger = logger.With(maps.ToKeyValueSlice(b.cfg.Tags)...)
	logger = b.addAppMetadata(logger, appGroupName)

	return logger, nil
}

// SetLevel changes the logging level at runtime.
func (b *LoggerBuilder) SetLevel(level slog.Level) {
	levelVar := cmp.Or(b.levelVar, slogx.GlobalLevel)
	levelVar.Set(level)
}

// GetLevel returns the current logging level.
func (b *LoggerBuilder) GetLevel() slog.Level {
	levelVar := cmp.Or(b.levelVar, slogx.GlobalLevel)
	return levelVar.Level()
}

// getWriter returns the appropriate writer based on output configuration.
func (b *LoggerBuilder) getWriter() *os.File {
	writers := map[config.LoggerConsoleOutput]*os.File{
		config.LoggerConsoleOutputStderr: os.Stderr,
	}
	return cmp.Or(writers[b.cfg.Output], os.Stdout)
}

// createHandler creates the base handler based on the output format.
func (b *LoggerBuilder) createHandler(writer *os.File, maskString string) slog.Handler {
	var level slog.Leveler = b.parseLevel(b.cfg.Level)
	levelVar := cmp.Or(b.levelVar, slogx.GlobalLevel)
	levelVar.Set(level.Level())
	level = levelVar

	timeFormat := cmp.Or(b.cfg.TimeFormat, time.RFC3339Nano)
	replaceAttr := slogx.MaskingReplaceAttr(b.cfg.SensitiveTags, maskString)

	opts := &slog.HandlerOptions{
		AddSource:   b.cfg.OutputSource,
		Level:       level,
		ReplaceAttr: replaceAttr,
	}

	customHandlersMu.RLock()
	customFactory, ok := customHandlers[b.cfg.OutputFormat]
	customHandlersMu.RUnlock()

	if ok {
		return customFactory(writer, b.cfg, opts)
	}

	switch b.cfg.OutputFormat {
	case config.LogFormatJSON:
		return slog.NewJSONHandler(writer, opts)
	default:
		noColor := !b.cfg.Colorized || !isatty.IsTerminal(writer.Fd())
		colorOpts := []colorized.Option{
			colorized.WithLevel(level),
			colorized.WithReplaceAttr(replaceAttr),
			colorized.WithTimeFormat(timeFormat),
			colorized.WithPrefixAttributeKey(b.prefixKey),
			colorized.WithAttributeColors(b.prefixColors),
		}
		colorOpts = slices.AppendIf(colorOpts, b.cfg.OutputSource, colorized.WithAddSource())
		colorOpts = slices.AppendIf(colorOpts, noColor, colorized.WithNoColor())
		return colorized.NewHandler(writer, colorOpts...)
	}
}

// wrapWithLeveledHandler adds per-subsystem level filtering to the handler chain.
// If no subsystem overrides are configured, the handler is not inserted.
func (b *LoggerBuilder) wrapWithLeveledHandler(handler slog.Handler) slog.Handler {
	if len(b.cfg.Subsystems) == 0 {
		return handler
	}

	levelVar := cmp.Or(b.levelVar, slogx.GlobalLevel)
	globalLevel := b.parseLevel(b.cfg.Level)
	minLevel := globalLevel

	subsystemLevels := make(map[string]slog.Level, len(b.cfg.Subsystems))
	for name, lvl := range b.cfg.Subsystems {
		parsed := b.parseLevel(lvl)
		subsystemLevels[name] = parsed
		if parsed < minLevel {
			minLevel = parsed
		}
	}

	levelVar.Set(minLevel)

	return leveled.NewHandler(handler,
		leveled.WithDefaultLevel(globalLevel),
		leveled.WithSubsystemLevels(subsystemLevels),
	)
}

// wrapWithPrefixedHandler adds prefix handling to the handler chain.
func (b *LoggerBuilder) wrapWithPrefixedHandler(handler slog.Handler) slog.Handler {
	formatter := prefixed.DefaultFormatter
	if b.cfg.OutputFormat == config.LogFormatJSON {
		formatter = prefixed.JsonFormatter
	}

	return prefixed.NewHandler(handler,
		prefixed.WithPrefix(b.prefixKey),
		prefixed.WithPrefixFormatter(formatter),
	)
}

// wrapWithMaskingHandler adds masking to the handler chain.
func (b *LoggerBuilder) wrapWithMaskingHandler(handler slog.Handler, maskString string) slog.Handler {
	opts := []masking.Option{}

	// Add default masks if enabled explicitly or if SensitiveTags are present (backward compatibility)
	opts = slices.AppendIf(opts,
		b.cfg.EnableDefaultMasks || len(b.cfg.SensitiveTags) > 0 || b.enableMasking,
		masking.WithDefaults())

	// Process mask rules from configuration
	for _, rule := range b.cfg.MaskRules {
		maskFunc, err := masking.CreateMask(rule.Type, rule.Params)
		if err != nil {
			// Log error but continue - don't fail the entire logger creation
			b.errs = append(b.errs, fmt.Errorf("invalid mask rule for %s: %w",
				cmp.Or(rule.Field, rule.Pattern), err))
			continue
		}

		if rule.Field != "" {
			opts = append(opts, masking.WithField(rule.Field, maskFunc))
		} else if rule.Pattern != "" {
			opts = append(opts, masking.WithPattern(rule.Pattern, maskFunc))
		}
	}

	// Backward compatibility with sensitiveTags - add them as additional field masks
	for _, tag := range b.cfg.SensitiveTags {
		opts = append(opts, masking.WithField(tag, masking.FullMask()))
	}

	// Set default mask string if provided
	opts = slices.AppendIf(opts, maskString != "", masking.WithDefaultMask(masking.FixedMask(maskString)))

	return masking.NewHandler(handler, opts...)
}

// addAppMetadata adds application metadata to the logger.
func (b *LoggerBuilder) addAppMetadata(logger *slog.Logger, appGroupName string) *slog.Logger {
	if b.appName == "" && b.appVersion == "" && b.serviceId == "" {
		return logger
	}

	var attrs []any
	attrs = slices.AppendNonEmpty(attrs, "name", b.appName)
	attrs = slices.AppendNonEmpty(attrs, "version", b.appVersion)
	attrs = slices.AppendNonEmpty(attrs, "sid", b.serviceId)

	if len(attrs) > 0 {
		return logger.With(slog.Group(appGroupName, attrs...))
	}
	return logger
}

// parseLevel converts config level to slog.Level.
func (b *LoggerBuilder) parseLevel(level config.LoggerLevel) slog.Level {
	switch level {
	case config.LoggerLevelDebug:
		return slog.LevelDebug
	case config.LoggerLevelInfo:
		return slog.LevelInfo
	case config.LoggerLevelWarning:
		return slog.LevelWarn
	case config.LoggerLevelError:
		return slog.LevelError
	default:
		return slog.LevelError
	}
}
