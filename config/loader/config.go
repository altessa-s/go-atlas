// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package loader

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"strings"
	"sync"

	"github.com/altessa-s/go-atlas/config/loader/backend"
	"github.com/altessa-s/go-atlas/config/loader/backend/yaml3"
	"github.com/altessa-s/go-atlas/core/types/nilcheck"

	loadersecrets "github.com/altessa-s/go-atlas/config/loader/secrets"
	coreerrs "github.com/altessa-s/go-atlas/core/errors"
	corestrings "github.com/altessa-s/go-atlas/core/text/strings"
)

const (
	// DefaultEnvDelimiter is the default delimiter for environment variable names.
	DefaultEnvDelimiter = "_"

	// DefaultEnvSectionDelimiter is the default delimiter for separating nested struct levels.
	DefaultEnvSectionDelimiter = "__"
)

const (
	minEnvKeyParts = 2 // Minimum parts for nested structures
)

var (
	// ErrBindDefaults is the error that returns when a default value cannot be bind.
	ErrBindDefaults = errors.New("bind defaults")

	// ErrBindEnv is the error that returns when an environment value cannot be bind.
	ErrBindEnv = errors.New("bind env")

	// ErrDecode is the error that returns when decode failed.
	ErrDecode = errors.New("decode error")
)

// Validator defines the interface for configuration validation.
// Implementations should validate configuration values and return an error
// if the configuration is invalid.
type Validator interface {
	Validate() error
}

// Defaulter defines the interface for setting default configuration values.
// Implementations should populate fields with appropriate default values.
type Defaulter interface {
	Default()
}

// Normalizer defines the interface for normalizing configuration values.
// Implementations should transform configuration values to their canonical form.
type Normalizer interface {
	Normalize()
}

// Config represents a configuration loader that can read from files and environment variables.
// It supports multiple backends (YAML, TOML) and provides validation, normalization,
// and default value handling.
//
// Example:
//
//	cfg := parser.New(nil, parser.WithPath("config.yaml"))
//	result, _ := cfg.Load(&MyConfig{})
type Config struct {
	options   *options
	files     *files
	conf      any
	backend   backend.Backend
	confType  reflect.Type
	fields    fields
	allowExts map[string]struct{}
	smx       sync.RWMutex
}

// New creates a new Config instance with the specified backend and options.
// If backend is nil, YAML backend is used as default.
// Options can be used to configure paths, environment prefixes, and other settings.
//
// Example:
//
//	cfg := parser.New(nil, parser.WithPath("config.yaml"), parser.WithEnvPrefix("APP_"))
func New(backend backend.Backend, option ...Option) *Config {
	conf := &Config{
		options:   newOptions(option...),
		files:     newFiles(),
		backend:   &yaml3.Backend{},
		allowExts: make(map[string]struct{}),
	}

	if backend != nil {
		conf.backend = backend
	}

	return conf
}

// Load loads configuration from files and environment variables into the provided struct.
// The conf parameter must be a pointer to a struct.
// Returns the populated configuration or an error if loading fails.
//
// Example:
//
//	cfg := &AppConfig{}
//	result, err := loader.Load(cfg)
func (cf *Config) Load(conf any) (any, error) {
	if err := assertStructPointer(conf); err != nil {
		return nil, err
	}

	cf.confType = reflect.TypeOf(conf)
	cf.conf = conf

	if reflect.ValueOf(conf).IsNil() {
		cf.conf = reflect.New(indirectType(reflect.TypeOf(conf))).Interface()
	}

	cf.smx.Lock()
	defer cf.smx.Unlock()

	if err := cf.load(); err != nil {
		return nil, err
	}

	return cf.conf, nil
}

// Config returns the current loaded configuration.
// This method is thread-safe and returns the configuration that was last successfully loaded.
func (cf *Config) Config() any {
	cf.smx.RLock()
	defer cf.smx.RUnlock()

	return cf.conf
}

// load performs the actual configuration loading process.
// It reads from files, applies defaults, loads environment variables, and runs validation.
func (cf *Config) load() error {
	var err error

	// load configuration from file(s).
	if cf.options.path != "" {
		if err = cf.loadFiles(); err != nil {
			return err
		}

		for f := range cf.files.All() {
			err = cf.loadAndDecode(f, cf.conf)
			if err != nil {
				return err
			}
		}
	}

	cf.fields = structFields(cf.conf)

	if err = cf.loadDefaultValues(); err != nil {
		return fmt.Errorf("%w: %w", ErrBindDefaults, err)
	}

	if err = cf.loadEnvs(); err != nil {
		return fmt.Errorf("%w: %w", ErrBindEnv, err)
	}

	// Re-apply default values to newly created structs from environment variables
	// This ensures that nested pointer structs created by setNestedFieldValue
	// receive their default values
	cf.fields = structFields(cf.conf)
	if err = cf.loadDefaultValues(); err != nil {
		return fmt.Errorf("%w: %w", ErrBindDefaults, err)
	}

	// Apply default values to structs inside maps
	if err = cf.applyDefaultsToMaps(cf.conf); err != nil {
		return fmt.Errorf("%w: %w", ErrBindDefaults, err)
	}

	// Expand secrets in all string fields using reflection
	if cf.options.secretsManager != nil {
		if err = cf.expandSecrets(); err != nil {
			return err
		}
	}

	if cv, ok := cf.conf.(Validator); ok {
		if err = cv.Validate(); err != nil {
			return coreerrs.Wrapf(err, "config validation failed for %s",
				reflect.TypeOf(cf.conf).Elem().Name())
		}
	}

	// Apply normalization to the root configuration object
	if n, ok := cf.conf.(Normalizer); ok {
		n.Normalize()
	}

	for f := range cf.fields.All() {
		if f.isStructPtr() && nilcheck.IsNotNilValue(f.value) {
			if n, ok := f.value.Interface().(Normalizer); ok {
				n.Normalize()
			}
		}
	}

	return nil
}

// expandSecrets walks through the configuration struct using reflection
// and expands all $__secret{namespace:key} placeholders in string fields.
func (cf *Config) expandSecrets() error {
	ctx, cancel := cf.options.getSecretsContext()
	defer cancel()

	expander := loadersecrets.New(cf.options.secretsManager)
	return expander.ExpandStruct(ctx, cf.conf)
}

// loadFiles loads one or multiple files if path is directory.
// It handles both single file paths and directory paths containing multiple config files.
func (cf *Config) loadFiles() (err error) {
	var basePath string

	basePath, err = fixPath(cf.options.path)
	if err != nil {
		return
	}

	var pathInfo os.FileInfo

	if pathInfo, err = os.Stat(basePath); err != nil {
		if os.IsNotExist(err) {
			// return error for non-existent paths
			return
		}

		return err
	}

	// if a directory path is specified, load all valid configuration files
	// from that directory.
	if pathInfo.IsDir() {
		return cf.loadFilesFromDir(basePath)
	}

	return cf.loadFile(basePath, pathInfo)
}

// loadFilesFromDir loads all valid configuration files from the specified directory.
// It iterates through directory contents and loads each valid configuration file.
func (cf *Config) loadFilesFromDir(basePath string) error {
	items, err := os.ReadDir(basePath)
	if err != nil {
		return err
	}

	for _, item := range items {
		var itemInfo os.FileInfo
		if itemInfo, err = item.Info(); err == nil {
			err = cf.loadFile(path.Join(basePath, item.Name()), itemInfo)
		}

		if err != nil {
			return err
		}
	}

	return nil
}

// loadFile loads a single configuration file if it's valid.
// It checks file validity and adds it to the internal files collection.
func (cf *Config) loadFile(filePath string, fileInfo os.FileInfo) (err error) {
	if fileInfo.IsDir() || !cf.isValidConfigFile(fileInfo.Name()) {
		return nil
	}

	_, fileName := filepath.Split(filePath)

	cf.files.add(&file{
		name:      fileName,
		path:      filePath,
		isSymlink: fileInfo.Mode()&os.ModeSymlink != 0,
		decoder:   cf.backend,
		sum:       "",
	})

	return
}

// envFieldName constructs the full environment variable name for a struct field.
// It first checks for an explicit 'env' tag when no prefix is configured, otherwise
// combines the configured prefix with the field's full name using the specified delimiter.
//
// The function uses __ (double underscore) as a section delimiter to separate nested
// struct levels, and converts camelCase field names to SCREAMING_SNAKE_CASE.
//
// Examples:
//   - GRPC.Interceptors.Cache.Enabled -> GRPC__INTERCEPTORS__CACHE__ENABLED
//   - GRPC.Interceptors.RealIp.Enabled -> GRPC__INTERCEPTORS__REAL_IP__ENABLED
//   - GRPC.Interceptors.RequestId.GenerateIfMissing -> GRPC__INTERCEPTORS__REQUEST_ID__GENERATE_IF_MISSING
func (cf *Config) envFieldName(fld *field) string {
	if fld.isStructPtr() {
		return ""
	}

	// Check if field has an explicit env tag first
	if envTag := fld.tags[envTagName]; envTag != "" {
		// If there's a prefix, replace any existing prefix in the tag or prepend it
		if cf.options.envPrefix != "" {
			// If tag starts with "APP_", replace it with the configured prefix
			if strings.HasPrefix(envTag, "APP_") {
				return cf.options.envPrefix + envTag[4:] // Remove "APP_" and prepend new prefix
			}
			// If tag doesn't already start with the configured prefix, prepend it
			if !strings.HasPrefix(envTag, cf.options.envPrefix) {
				return cf.options.envPrefix + envTag
			}
		}
		return envTag
	}

	// Split the field full name by dots (struct hierarchy)
	parts := strings.Split(fld.fullName, ".")

	// Convert each part from camelCase to SCREAMING_SNAKE_CASE
	convertedParts := make([]string, len(parts))
	for i, part := range parts {
		convertedParts[i] = corestrings.ToScreamingSnakeCase(part)
	}

	// Join parts with configured section delimiter
	envName := strings.Join(convertedParts, cf.options.envSectionDelimiter)

	// Prepend prefix if configured
	if cf.options.envPrefix != "" {
		envName = cf.options.envPrefix + envName
	}

	return envName
}

// isValidConfigFile checks if the given filename has a valid configuration file extension.
// It uses the backend's supported file extensions to determine validity.
func (cf *Config) isValidConfigFile(fileName string) bool {
	// Initialize allowExts if empty
	if len(cf.allowExts) == 0 {
		for _, ext := range cf.backend.FileExtensions() {
			cf.allowExts["."+corestrings.InternLowerString(strings.TrimLeft(ext, "."))] = struct{}{}
		}
	}

	_, ok := cf.allowExts[corestrings.InternLowerString(filepath.Ext(fileName))]

	return ok
}

// fixPath normalizes path to config file.
// It expands '~' at the beginning of path to the home directory.
// It expands '$NAME' to the corresponding OS environment variable value.
func fixPath(filePath string) (string, error) {
	if strings.HasPrefix(filePath, "~") {
		filePath = filePath[1:]
		filePath = path.Join(userHomeDir() + filePath)

		goto abs
	}

	if strings.Contains(filePath, "$") {
		filePath = substituteEnvVariables(filePath)
	}

abs:
	if !filepath.IsAbs(filePath) {
		var err error
		if filePath, err = filepath.Abs(filePath); err != nil {
			return "", err
		}
	}

	return filepath.Clean(filePath), nil
}
