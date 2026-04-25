// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config

import (
	ozzo_rules "github.com/altessa-s/ozzo-rules"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// Default values for Logger configuration.
const (
	defaultLoggerLevel        = LoggerLevelError
	defaultLoggerOutput       = LoggerConsoleOutputStdout
	defaultLoggerOutputFormat = LogFormatText
	defaultLoggerMaskString   = "****"
	defaultLoggerAppGroupName = "app"
	defaultLoggerBufferSize   = 100
	defaultLoggerBypassLevel  = LoggerLevelError
)

// LoggerConsoleOutput represents the console output destination for logging.
type LoggerConsoleOutput = string

const (
	// LoggerConsoleOutputStdout directs log output to standard output.
	LoggerConsoleOutputStdout LoggerConsoleOutput = "stdout"
	// LoggerConsoleOutputStderr directs log output to standard error.
	LoggerConsoleOutputStderr LoggerConsoleOutput = "stderr"
)

// LoggerLevel represents the minimum logging level for message output.
type LoggerLevel = string

const (
	// LoggerLevelError logs only error messages.
	LoggerLevelError LoggerLevel = "error"
	// LoggerLevelWarning logs warning and error messages.
	LoggerLevelWarning LoggerLevel = "warning"
	// LoggerLevelInfo logs informational, warning, and error messages.
	LoggerLevelInfo LoggerLevel = "info"
	// LoggerLevelDebug logs all messages including debug information.
	LoggerLevelDebug LoggerLevel = "debug"
	// LoggerLevelNone disables all logging output.
	LoggerLevelNone LoggerLevel = "none"
)

// LogFormat represents the output format for log messages.
type LogFormat = string

const (
	// LogFormatText outputs log messages in human-readable text format.
	LogFormatText LogFormat = "text"
	// LogFormatJSON outputs log messages in structured JSON format.
	LogFormatJSON LogFormat = "json"
)

// LoggerBuffer configures asynchronous buffered logging.
// When enabled, log records are written in the background for better performance.
// Records at or above BypassLevel are still written synchronously.
type LoggerBuffer struct {
	// Enabled activates asynchronous buffered logging
	Enabled bool `yaml:"enabled"`
	// Size is the number of log records the buffer can hold (default: 100)
	Size int `yaml:"size" default:"100"`
	// BypassLevel is the minimum level that bypasses the buffer and writes synchronously (default: error)
	BypassLevel LoggerLevel `yaml:"bypassLevel" default:"error"`
}

// DefaultLoggerBuffer returns a LoggerBuffer with default values.
func DefaultLoggerBuffer() LoggerBuffer {
	return LoggerBuffer{
		Size:        defaultLoggerBufferSize,
		BypassLevel: defaultLoggerBypassLevel,
	}
}

// Validate checks that the LoggerBuffer configuration is valid.
func (b *LoggerBuffer) Validate() error {
	return ValidateStruct(b,
		validation.Field(&b.BypassLevel, ozzo_rules.OneOf(LoggerLevelError, LoggerLevelWarning, LoggerLevelInfo,
			LoggerLevelDebug, LoggerLevelNone)),
	)
}

// Logger configures logging behavior for applications.
// It controls log levels, output destinations, formatting, and metadata handling.
//
// Example:
//
//	logger := &config.Logger{
//		Level:        config.LoggerLevelInfo,
//		OutputFormat: config.LogFormatJSON,
//	}
type Logger struct {
	// Level sets the minimum logging level for message output
	Level LoggerLevel `yaml:"level" default:"error"`
	// Tags contains key-value pairs to include with all log messages
	Tags map[string]string `yaml:"tags"`
	// TimeFormat specifies the format for timestamps in log messages
	TimeFormat string `yaml:"timeFormat"`
	// Colorized enables colored output for console logging
	Colorized bool `yaml:"colorized"`
	// Output specifies whether to write to stdout or stderr
	Output LoggerConsoleOutput `yaml:"output" default:"stdout"`
	// OutputFormat controls whether logs are in text or JSON format
	OutputFormat LogFormat `yaml:"outputFormat" default:"text"`
	// SensitiveTags lists tag keys that should be redacted in log output.
	// A non-empty list also auto-enables the advanced masking handler in
	// observability/slog/factory, which applies a curated default
	// sensitive-field set (password, token, secret, *_key, ...) and
	// nested-group masking on top of these explicit tags.
	SensitiveTags []string `yaml:"sensitiveTags"`
	// MaskString is the string used for masking sensitive fields
	MaskString string `yaml:"maskString" default:"****"`
	// AppGroupName is the group name for application metadata
	AppGroupName string `yaml:"appGroupName" default:"app"`
	// OutputSource includes source file and line information in log messages
	OutputSource bool `yaml:"outputSource"`
	// Buffer contains asynchronous buffered logging configuration
	Buffer LoggerBuffer `yaml:"buffer"`
	// Subsystems overrides per-subsystem log levels.
	// Keys are subsystem names matching the "subsystem" attribute.
	Subsystems map[string]LoggerLevel `yaml:"subsystems"`
}

// DefaultLogger returns a Logger configuration with default values.
func DefaultLogger() Logger {
	return Logger{
		Level:        defaultLoggerLevel,
		Output:       defaultLoggerOutput,
		OutputFormat: defaultLoggerOutputFormat,
		MaskString:   defaultLoggerMaskString,
		AppGroupName: defaultLoggerAppGroupName,
		Buffer:       DefaultLoggerBuffer(),
	}
}

// Validate checks that the Logger configuration is valid.
// It ensures that all enum-type fields contain supported values.
func (l *Logger) Validate() error {
	return ValidateStruct(l,
		validation.Field(&l.Level, ozzo_rules.OneOf(LoggerLevelError, LoggerLevelWarning, LoggerLevelInfo,
			LoggerLevelDebug, LoggerLevelNone)),
		validation.Field(&l.Output, ozzo_rules.OneOf(LoggerConsoleOutputStdout, LoggerConsoleOutputStderr)),
		validation.Field(&l.Buffer),
		validation.Field(&l.Subsystems, validation.Each(ozzo_rules.OneOf(LoggerLevelError, LoggerLevelWarning,
			LoggerLevelInfo, LoggerLevelDebug, LoggerLevelNone))),
	)
}
