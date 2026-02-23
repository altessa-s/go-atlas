// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"cmp"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/mattn/go-isatty"

	"github.com/altessa-s/go-atlas/config"
	"github.com/altessa-s/go-atlas/core/collections/maps"
	"github.com/altessa-s/go-atlas/core/collections/slices"
	"github.com/altessa-s/go-atlas/core/runtime"
	"github.com/altessa-s/go-atlas/observability/slog/handler/buffered"
	"github.com/altessa-s/go-atlas/observability/slog/handler/colorized"
	"github.com/altessa-s/go-atlas/observability/slog/handler/masking"
	"github.com/altessa-s/go-atlas/observability/slog/handler/prefixed"

	corefactory "github.com/altessa-s/go-atlas/core/factory"
	slogx "github.com/altessa-s/go-atlas/observability/slog"
)

// Factory creates [slog.Logger] instances from [config.Logger] configuration.
// It composes handlers from the colorized, prefixed, masking, and buffered
// sub-packages based on config flags.
type Factory struct {
	corefactory.Base
	cfg *options
}

// HandlerFactory creates a [slog.Handler] for a specific log format.
// Register custom factories via [RegisterHandler].
type HandlerFactory func(w io.Writer, cfg *config.Logger, opts *slog.HandlerOptions) slog.Handler

var (
	// customHandlers is a registry of custom log handler factories.
	customHandlers   = make(map[config.LogFormat]HandlerFactory)
	customHandlersMu sync.RWMutex
)

// RegisterHandler registers a custom [HandlerFactory] for the given log format.
// Safe for concurrent use.
func RegisterHandler(format config.LogFormat, factory HandlerFactory) {
	customHandlersMu.Lock()
	defer customHandlersMu.Unlock()
	customHandlers[format] = factory
}

// New creates a new Factory with the given options.
func New(opts ...Option) *Factory {
	cfg := newOptions(opts...)
	return &Factory{
		Base: corefactory.NewBase(nil),
		cfg:  cfg,
	}
}

// CreateLoggerFromConfig creates a slog.Logger from configuration.
func (f *Factory) CreateLoggerFromConfig(cfg *config.Logger) (*slog.Logger, error) {
	if cfg == nil {
		return nil, fmt.Errorf("configuration is required")
	}
	if cfg.Level == config.LoggerLevelNone {
		return slog.New(slog.DiscardHandler), nil
	}

	maskString := cmp.Or(cfg.MaskString, "****")
	appGroupName := cmp.Or(cfg.AppGroupName, "app")

	writer := f.getWriter(cfg.Output)
	handler := f.createHandler(cfg, writer, maskString)

	// Wrap with prefixed handler
	handler = f.wrapWithPrefixedHandler(handler, cfg.OutputFormat)

	// Wrap with masking handler if enabled
	if f.cfg.enableMasking && len(cfg.SensitiveTags) > 0 {
		handler = f.wrapWithMaskingHandler(handler, cfg.SensitiveTags, maskString)
	}

	// Wrap with buffered handler if enabled
	if cfg.Buffered {
		handler = buffered.NewHandler(handler,
			buffered.WithBufferSize(cfg.BufferSize),
			buffered.WithBypassLevel(f.parseLevel(cfg.BypassLevel)),
		)
	}

	logger := slog.New(handler)

	// Register shutdown hook for buffered logger
	if cfg.Buffered {
		runtime.OnShutdown(func(ctx context.Context) error {
			return slogx.Shutdown(ctx, logger)
		})
	}

	// Add custom tags
	logger = logger.With(maps.ToKeyValueSlice(cfg.Tags)...)

	// Add app metadata if configured
	logger = f.addAppMetadata(logger, appGroupName)

	return logger, nil
}

// CreateColorizedHandler creates a colorized handler for terminal output.
func (f *Factory) CreateColorizedHandler(w io.Writer, opts ...colorized.Option) slog.Handler {
	return colorized.NewHandler(w, opts...)
}

// CreateJSONHandler creates a standard JSON handler.
func (f *Factory) CreateJSONHandler(w io.Writer, opts *slog.HandlerOptions) slog.Handler {
	return slog.NewJSONHandler(w, opts)
}

// CreateTextHandler creates a standard text handler.
func (f *Factory) CreateTextHandler(w io.Writer, opts *slog.HandlerOptions) slog.Handler {
	return slog.NewTextHandler(w, opts)
}

// CreatePrefixedHandler wraps a handler with prefix extraction.
func (f *Factory) CreatePrefixedHandler(inner slog.Handler, opts ...prefixed.Option) slog.Handler {
	return prefixed.NewHandler(inner, opts...)
}

// CreateMaskingHandler wraps a handler with sensitive field masking.
func (f *Factory) CreateMaskingHandler(inner slog.Handler, opts ...masking.Option) slog.Handler {
	return masking.NewHandler(inner, opts...)
}

// getWriter returns the appropriate writer based on output configuration.
func (f *Factory) getWriter(output config.LoggerConsoleOutput) *os.File {
	writers := map[config.LoggerConsoleOutput]*os.File{
		config.LoggerConsoleOutputStderr: os.Stderr,
	}
	return cmp.Or(writers[output], os.Stdout)
}

// createHandler creates the base handler based on the output format.
func (f *Factory) createHandler(cfg *config.Logger, writer *os.File, maskString string) slog.Handler {
	var level slog.Leveler = f.parseLevel(cfg.Level)
	levelVar := cmp.Or(f.cfg.levelVar, slogx.GlobalLevel)
	levelVar.Set(level.Level())
	level = levelVar

	timeFormat := cmp.Or(cfg.TimeFormat, time.RFC3339Nano)
	replaceAttr := slogx.MaskingReplaceAttr(cfg.SensitiveTags, maskString)

	opts := &slog.HandlerOptions{
		AddSource:   cfg.OutputSource,
		Level:       level,
		ReplaceAttr: replaceAttr,
	}

	// Check for registered custom handlers first
	customHandlersMu.RLock()
	customFactory, ok := customHandlers[cfg.OutputFormat]
	customHandlersMu.RUnlock()

	if ok {
		return customFactory(writer, cfg, opts)
	}

	switch cfg.OutputFormat {
	case config.LogFormatJSON:
		return slog.NewJSONHandler(writer, opts)
	default: // text format with colorization
		noColor := !cfg.Colorized || !isatty.IsTerminal(writer.Fd())
		colorOpts := []colorized.Option{
			colorized.WithLevel(level),
			colorized.WithReplaceAttr(replaceAttr),
			colorized.WithTimeFormat(timeFormat),
			colorized.WithPrefixAttributeKey(f.cfg.prefixKey),
			colorized.WithAttributeColors(f.cfg.prefixColors),
		}
		colorOpts = slices.AppendIf(colorOpts, cfg.OutputSource, colorized.WithAddSource())
		colorOpts = slices.AppendIf(colorOpts, noColor, colorized.WithNoColor())
		return colorized.NewHandler(writer, colorOpts...)
	}
}

// wrapWithPrefixedHandler adds prefix handling to the handler chain.
func (f *Factory) wrapWithPrefixedHandler(handler slog.Handler, format config.LogFormat) slog.Handler {
	formatter := prefixed.DefaultFormatter
	if format == config.LogFormatJSON {
		formatter = prefixed.JsonFormatter
	}

	return prefixed.NewHandler(handler,
		prefixed.WithPrefix(f.cfg.prefixKey),
		prefixed.WithPrefixFormatter(formatter),
	)
}

// wrapWithMaskingHandler adds masking to the handler chain.
func (f *Factory) wrapWithMaskingHandler(handler slog.Handler, sensitiveTags []string, maskString string) slog.Handler {
	opts := []masking.Option{masking.WithDefaults()}
	if maskString != "" {
		opts = append(opts, masking.WithDefaultMask(masking.FixedMask(maskString)))
	}

	for _, tag := range sensitiveTags {
		opts = append(opts, masking.WithField(tag, masking.FullMask()))
	}

	return masking.NewHandler(handler, opts...)
}

// addAppMetadata adds application metadata to the logger.
func (f *Factory) addAppMetadata(logger *slog.Logger, appGroupName string) *slog.Logger {
	if f.cfg.appName == "" && f.cfg.appVersion == "" && f.cfg.serviceId == "" {
		return logger
	}

	var attrs []any
	attrs = slices.AppendNonEmpty(attrs, "name", f.cfg.appName)
	attrs = slices.AppendNonEmpty(attrs, "version", f.cfg.appVersion)
	attrs = slices.AppendNonEmpty(attrs, "sid", f.cfg.serviceId)

	if len(attrs) > 0 {
		return logger.With(slog.Group(appGroupName, attrs...))
	}
	return logger
}

// SetLevel changes the logging level at runtime.
func (f *Factory) SetLevel(level slog.Level) {
	levelVar := cmp.Or(f.cfg.levelVar, slogx.GlobalLevel)
	levelVar.Set(level)
}

// GetLevel returns the current logging level.
func (f *Factory) GetLevel() slog.Level {
	levelVar := cmp.Or(f.cfg.levelVar, slogx.GlobalLevel)
	return levelVar.Level()
}

// parseLevel converts config level to slog.Level.
func (f *Factory) parseLevel(level config.LoggerLevel) slog.Level {
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
