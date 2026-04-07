// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package convert

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/MakeNowJust/heredoc"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/altessa-s/go-atlas/core/io/files"
	"github.com/altessa-s/go-atlas/core/runtime/appinfo"
	"github.com/altessa-s/go-atlas/core/runtime/concurrency"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
	corestrings "github.com/altessa-s/go-atlas/core/text/strings"
)

const (
	// File extensions
	extensionYAML = ".yaml"
	extensionYML  = ".yml"
	extensionTOML = ".toml"
	extensionEnv  = ".env"
	extensionMD   = ".md"

	// Format types
	formatYAML = "yaml"
	formatTOML = "toml"
	formatEnv  = "env"
	formatMD   = "md"

	filePermission = 0o644

	// Parallel processing threshold
	parallelProcessingThreshold = 5 // Minimum number of files to use parallel processing
)

// Command holds the state for a single invocation of "goconfig convert".
// It embeds [cobra.Command] and stores the resolved flag values.
type Command struct {
	*cobra.Command
	fromPath      string
	toPath        string
	fromFormat    string
	toFormat      string
	claudeAPIKey  string
	parseComments bool
	useClaude     bool
}

// New returns a cobra.Command that performs configuration file conversion.
// The returned command requires --from and --to flags and supports optional
// --from-format, --to-format, --parse-comments, --use-claude, and
// --claude-api-key flags.
func New() *cobra.Command {
	cmd := &Command{
		Command: &cobra.Command{
			Use:   "convert",
			Short: "Convert configuration files between formats",
			Long: heredoc.Doc(`
				Convert configuration files between different formats.

				Supported formats:
				- YAML (.yaml, .yml)
				- TOML (.toml)
				- .env (environment variables)
				- Markdown (.md) - documentation tables

				The converter automatically detects formats based on file extensions,
				or you can specify them explicitly using --from-format and --to-format flags.

				Conversion rules:
				- Nested structures use __ (double underscore) as delimiter
				- camelCase field names convert to SCREAMING_SNAKE_CASE
				- Example: GRPC.Interceptors.RealIp -> GRPC__INTERCEPTORS__REAL_IP
			`),
			Example: heredoc.Doc(`
				# Convert YAML to .env
				goconfig convert --from config.yaml --to .env

				# Convert .env to YAML
				goconfig convert --from .env --to config.yaml

				# Convert TOML to .env
				goconfig convert --from config.toml --to .env

				# Convert all files in directory to .env (merges all configs)
				goconfig convert --from ./configs/ --to merged.env

				# Parse commented lines (YAML comments -> commented env vars)
				goconfig convert --from config.yaml --to .env --parse-comments

				# Parse commented env vars and include them in config
				goconfig convert --from .env --to config.yaml --parse-comments

				# Specify format explicitly
				goconfig convert --from config.txt --to output.txt --from-format yaml --to-format toml

				# Use short flags
				goconfig convert -f config.yml -t production.env
			`),
		},
	}

	cmd.configure()

	return cmd.Command
}

// configure sets up the convert command with flags and run function.
func (c *Command) configure() {
	c.Flags().StringVarP(&c.fromPath, "from", "f", "", heredoc.Doc(`
		Source configuration file or directory path.

		If a directory is specified, all configuration files matching the
		source format will be loaded and merged into a single output.
		Required.
	`))

	c.Flags().StringVarP(&c.toPath, "to", "t", "", heredoc.Doc(`
		Target configuration file path.

		The output format is determined by the file extension unless
		explicitly specified with --to-format.
		Required.
	`))

	c.Flags().StringVar(&c.fromFormat, "from-format", "", heredoc.Doc(`
		Source format: yaml, toml, env, or md.

		If not specified, the format will be auto-detected from the
		file extension. Required when using non-standard extensions.
	`))

	c.Flags().StringVar(&c.toFormat, "to-format", "", heredoc.Doc(`
		Target format: yaml, toml, env, or md.

		If not specified, the format will be auto-detected from the
		file extension. Required when using non-standard extensions.
	`))

	c.Flags().BoolVar(&c.parseComments, "parse-comments", false, heredoc.Doc(`
		Parse commented lines in configuration files.

		When converting YAML/TOML to .env, fields with inline comments
		will be output as commented environment variables.
		When converting .env to YAML/TOML, commented lines (starting with #)
		will be included in the output.
	`))

	c.Flags().BoolVar(&c.useClaude, "use-claude", false, heredoc.Doc(`
		Use Claude API to generate field descriptions.

		When enabled, Claude will generate descriptions for configuration
		fields in markdown output. Requires --claude-api-key to be set.
	`))

	c.Flags().StringVar(&c.claudeAPIKey, "claude-api-key", "", heredoc.Doc(`
		Claude API key for AI-powered description generation.

		Required when --use-claude is enabled. Can also be set via
		ANTHROPIC_API_KEY environment variable.
	`))

	// Mark flags as required
	// Errors can only occur if the flag doesn't exist, which is a programming error
	// that would be caught during development. Silent ignore is safe here.
	_ = c.MarkFlagRequired("from") //nolint:errcheck
	_ = c.MarkFlagRequired("to")   //nolint:errcheck

	// Set pre-run hook for validation
	c.PersistentPreRunE = c.preRun

	// Set post-run hook for cleanup
	c.PersistentPostRun = c.postRun

	c.RunE = c.run
}

// preRun performs validation before command execution.
func (c *Command) preRun(cmd *cobra.Command, _ []string) error {
	// Check for Claude API key from environment variable if not set via flag
	if c.useClaude {
		if c.claudeAPIKey == "" {
			c.claudeAPIKey = appinfo.Env(appinfo.EnvAnthropicAPIKey)
		}
		if c.claudeAPIKey == "" {
			return fmt.Errorf("--claude-api-key is required when --use-claude is enabled (or set %s environment variable)", appinfo.EnvAnthropicAPIKey)
		}
	}

	return nil
}

// postRun performs cleanup after command execution.
func (c *Command) postRun(_ *cobra.Command, _ []string) {}

// run executes the convert command.
func (c *Command) run(cmd *cobra.Command, args []string) error {
	// Validate input paths
	isDir, err := c.validatePaths()
	if err != nil {
		return err
	}

	// Determine source and target formats
	fromFormat := c.fromFormat
	if fromFormat == "" {
		if isDir {
			// For directories, we need explicit format or detect from first file
			fromFormat, err = c.detectDirectoryFormat()
			if err != nil {
				return err
			}
		} else {
			fromFormat = c.detectFormat(c.fromPath)
		}
	}

	toFormat := c.toFormat
	if toFormat == "" {
		toFormat = c.detectFormat(c.toPath)
	}

	// Validate formats
	if err := c.validateFormat(fromFormat, "from"); err != nil {
		return err
	}
	if err := c.validateFormat(toFormat, "to"); err != nil {
		return err
	}

	// Perform conversion
	sourceDesc := c.fromPath
	if isDir {
		sourceDesc = c.fromPath + " (directory)"
	}
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Converting %s (%s) -> %s (%s)...\n",
		sourceDesc, fromFormat, c.toPath, toFormat)

	return c.performConversion(fromFormat, toFormat, isDir)
}

// validatePaths validates that input and output paths are valid.
// Returns true if source is a directory, false if it's a file.
func (c *Command) validatePaths() (bool, error) {
	// Check if source path exists
	sourceInfo, err := os.Stat(c.fromPath)
	if os.IsNotExist(err) {
		return false, fmt.Errorf("source path does not exist: %s", c.fromPath)
	}
	if err != nil {
		return false, coreerrs.WrapOperation(err, "stat source path")
	}

	// Check if target directory exists
	targetDir := filepath.Dir(c.toPath)
	if targetDir != "." && targetDir != "" {
		if _, err := os.Stat(targetDir); os.IsNotExist(err) {
			return false, fmt.Errorf("target directory does not exist: %s", targetDir)
		}
	}

	return sourceInfo.IsDir(), nil
}

// detectFormat detects the file format based on extension.
func (c *Command) detectFormat(path string) string {
	ext := corestrings.InternLowerString(filepath.Ext(path))

	switch ext {
	case extensionYAML, extensionYML:
		return formatYAML
	case extensionTOML:
		return formatTOML
	case extensionEnv:
		return formatEnv
	case extensionMD:
		return formatMD
	default:
		return ""
	}
}

// detectDirectoryFormat detects the format from the first valid config file in directory.
func (c *Command) detectDirectoryFormat() (string, error) {
	for entry, err := range files.Walk(c.fromPath, files.WithFileTypes(files.FileTypeRegular)) {
		if err != nil {
			return "", coreerrs.WrapOperation(err, "read directory")
		}

		if format := c.detectFormat(entry.Name()); format != "" {
			return format, nil
		}
	}

	return "", fmt.Errorf("no valid config files found in directory: %s", c.fromPath)
}

// validateFormat validates that the format is supported.
func (c *Command) validateFormat(format, formatType string) error {
	if format == "" {
		return fmt.Errorf("%s format could not be detected - use --%s-format flag", formatType, formatType)
	}

	validFormats := []string{formatYAML, formatTOML, formatEnv, formatMD}
	if slices.Contains(validFormats, format) {
		return nil
	}

	return fmt.Errorf("unsupported %s format: %s (supported: %s)",
		formatType, format, strings.Join(validFormats, ", "))
}

// performConversion performs the actual conversion based on formats.
func (c *Command) performConversion(fromFormat, toFormat string, isDir bool) error {
	// If source is a directory, load and merge all files first
	var mergedData map[string]any
	var err error

	if isDir {
		// For markdown conversion, always uncomment YAML files
		uncommentYAML := toFormat == formatMD
		mergedData, err = c.loadAndMergeDirectory(fromFormat, uncommentYAML)
		if err != nil {
			return err
		}
	}

	// Config format (YAML/TOML) to .env
	if (fromFormat == formatYAML || fromFormat == formatTOML) && toFormat == formatEnv {
		converter := NewConfigToEnvConverter()
		converter.SetParseComments(c.parseComments)
		if isDir {
			// When converting from directory with parse-comments, mark all keys as commented
			var commentedKeys map[string]bool
			if c.parseComments {
				commentedKeys = make(map[string]bool)
				converter.SetParseComments(true)
				markAllAsCommented(mergedData, "", commentedKeys)
			}
			return converter.ConvertDataWithComments(mergedData, commentedKeys, c.toPath)
		}
		return converter.Convert(c.fromPath, c.toPath, fromFormat)
	}

	// .env to config format (YAML/TOML)
	if fromFormat == formatEnv && (toFormat == formatYAML || toFormat == formatTOML) {
		if isDir {
			return fmt.Errorf(".env format does not support directory input")
		}
		converter := NewEnvToConfigConverter()
		converter.SetParseComments(c.parseComments)
		return converter.Convert(c.fromPath, c.toPath, toFormat)
	}

	// Config format to config format (YAML <-> TOML)
	if (fromFormat == formatYAML || fromFormat == formatTOML) &&
		(toFormat == formatYAML || toFormat == formatTOML) {
		converter := NewConfigToConfigConverter()
		if isDir {
			return converter.ConvertData(mergedData, c.toPath, toFormat)
		}
		return converter.Convert(c.fromPath, c.toPath, fromFormat, toFormat)
	}

	// Config format (YAML/TOML) to Markdown
	if (fromFormat == formatYAML || fromFormat == formatTOML) && toFormat == formatMD {
		converter := NewConfigToMarkdownConverter()
		if c.useClaude {
			converter.SetClaudeAPI(c.claudeAPIKey)
		}
		if isDir {
			return converter.ConvertData(mergedData, c.toPath)
		}
		return converter.Convert(c.fromPath, c.toPath, fromFormat)
	}

	return fmt.Errorf("unsupported conversion: %s to %s", fromFormat, toFormat)
}

// loadAndMergeDirectory loads all config files from a directory and deep-merges
// them into a single map. When the number of matching files is below
// parallelProcessingThreshold the files are processed sequentially; otherwise
// they are loaded concurrently via [concurrency.ProcessCollect].
// If uncommentYAML is true, commented YAML blocks are uncommented before parsing.
func (c *Command) loadAndMergeDirectory(format string, uncommentYAML bool) (map[string]any, error) {
	names, err := c.collectConfigFileNames(format)
	if err != nil {
		return nil, err
	}

	if len(names) < parallelProcessingThreshold {
		return c.loadAndMergeSequential(names, format, uncommentYAML)
	}

	return c.loadAndMergeParallel(names, format, uncommentYAML)
}

// collectConfigFileNames returns the base names of regular files in c.fromPath
// whose extension matches the given format.
func (c *Command) collectConfigFileNames(format string) ([]string, error) {
	var names []string
	for entry, err := range files.Walk(c.fromPath,
		files.WithFileTypes(files.FileTypeRegular),
		files.WithExtensions(extensionsForFormat(format)...),
	) {
		if err != nil {
			return nil, coreerrs.WrapOperation(err, "read directory")
		}
		names = append(names, entry.Name())
	}
	return names, nil
}

// extensionsForFormat returns the file extensions associated with a config format.
func extensionsForFormat(format string) []string {
	switch format {
	case formatYAML:
		return []string{extensionYAML, extensionYML}
	case formatTOML:
		return []string{extensionTOML}
	case formatEnv:
		return []string{extensionEnv}
	case formatMD:
		return []string{extensionMD}
	}
	return nil
}

// isPathWithinDir reports whether path is inside dir after cleaning both.
// It uses [filepath.Rel] to avoid prefix-based path traversal bypasses
// (e.g., /tmp/a vs /tmp/ab).
func isPathWithinDir(dir, path string) bool {
	cleanDir := filepath.Clean(dir)
	cleanPath := filepath.Clean(path)

	rel, err := filepath.Rel(cleanDir, cleanPath)
	if err != nil {
		return false
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false
	}
	return true
}

// loadAndMergeSequential loads and merges config files sequentially.
func (c *Command) loadAndMergeSequential(names []string, format string, uncommentYAML bool) (map[string]any, error) {
	merged := make(map[string]any)

	for _, name := range names {
		filePath := filepath.Join(c.fromPath, name)

		// Security: validate path
		if !isPathWithinDir(c.fromPath, filePath) {
			return nil, fmt.Errorf("invalid file path: %s", name)
		}

		fileData, err := c.loadConfigFile(filePath, format, uncommentYAML)
		if err != nil {
			return nil, coreerrs.Wrapf(err, "failed to load %s", name)
		}

		c.deepMerge(merged, fileData)
	}

	if len(merged) == 0 {
		return nil, fmt.Errorf("no valid config files found in directory")
	}

	return merged, nil
}

// loadAndMergeParallel loads and merges config files in parallel.
func (c *Command) loadAndMergeParallel(names []string, format string, uncommentYAML bool) (map[string]any, error) {
	type fileResult struct {
		data map[string]any
		name string
	}

	if len(names) == 0 {
		return nil, fmt.Errorf("no valid config files found in directory")
	}

	results, err := concurrency.ProcessCollect(context.Background(), names, func(ctx context.Context, name string) (fileResult, error) {
		filePath := filepath.Join(c.fromPath, name)

		// Security: validate path
		if !isPathWithinDir(c.fromPath, filePath) {
			return fileResult{}, fmt.Errorf("invalid file path: %s", name)
		}

		fileData, err := c.loadConfigFile(filePath, format, uncommentYAML)
		if err != nil {
			return fileResult{}, err
		}

		return fileResult{name: name, data: fileData}, nil
	}, concurrency.BatchConfig[string]{
		StopOnError: true, // Stop on first error
	})

	if err != nil {
		return nil, coreerrs.WrapOperation(err, "load config files")
	}

	merged := make(map[string]any)
	for _, result := range results {
		c.deepMerge(merged, result.data)
	}

	if len(merged) == 0 {
		return nil, fmt.Errorf("no valid config files found in directory")
	}

	return merged, nil
}

// loadConfigFile loads a single config file and returns its data.
// #nosec G304 -- CLI tool, path from command-line arguments
func (c *Command) loadConfigFile(path, format string, uncommentYAML bool) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, coreerrs.WrapOperation(err, "read file")
	}

	var configData map[string]any

	switch format {
	case formatYAML:
		// If parse-comments is enabled or uncommentYAML is true, use the converter's uncomment logic
		if c.parseComments || uncommentYAML {
			converter := NewConfigToEnvConverter()
			uncommentedData := converter.UncommentYAML(data)
			if err := yaml.Unmarshal(uncommentedData, &configData); err != nil {
				return nil, coreerrs.WrapOperation(err, "parse YAML")
			}
		} else {
			if err := yaml.Unmarshal(data, &configData); err != nil {
				return nil, coreerrs.WrapOperation(err, "parse YAML")
			}
			// If the file parsed to empty, it may be a fully-commented template.
			// Auto-detect and uncomment to extract template values.
			if len(configData) == 0 {
				converter := NewConfigToEnvConverter()
				if converter.isFullyCommented(data) {
					uncommentedData := converter.UncommentYAML(data)
					if err := yaml.Unmarshal(uncommentedData, &configData); err != nil {
						return nil, coreerrs.WrapOperation(err, "parse uncommented YAML")
					}
				}
			}
		}
	case formatTOML:
		if err := toml.Unmarshal(data, &configData); err != nil {
			return nil, coreerrs.WrapOperation(err, "parse TOML")
		}
	default:
		return nil, fmt.Errorf("unsupported format: %s", format)
	}

	return configData, nil
}

// deepMerge recursively merges src into dst.
func (c *Command) deepMerge(dst, src map[string]any) {
	for key, srcValue := range src {
		if dstValue, exists := dst[key]; exists {
			// If both values are maps, merge recursively
			if dstMap, dstOk := dstValue.(map[string]any); dstOk {
				if srcMap, srcOk := srcValue.(map[string]any); srcOk {
					c.deepMerge(dstMap, srcMap)
					continue
				}
			}
		}
		// Otherwise, overwrite with src value
		dst[key] = srcValue
	}
}

// markAllAsCommented recursively marks all keys in config as commented.
func markAllAsCommented(data map[string]any, prefix string, commentedKeys map[string]bool) {
	// Use a converter instance to access the parser package function
	converter := NewConfigToEnvConverter()
	markAllAsCommentedRecursive(data, prefix, commentedKeys, converter)
}

// markAllAsCommentedRecursive is the recursive helper for markAllAsCommented.
func markAllAsCommentedRecursive(data map[string]any, prefix string, commentedKeys map[string]bool, converter *ConfigToEnvConverter) {
	// Flatten the entire config to get all env var names that will be generated
	envVars := converter.FlattenConfig(data, prefix)
	for envKey := range envVars {
		commentedKeys[envKey] = true
	}
}
