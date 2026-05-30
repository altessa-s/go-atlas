// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package commands

import (
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/spf13/cobra"

	"github.com/altessa-s/go-atlas/tools/codegen/optgen/internal/generator"
	"github.com/altessa-s/go-atlas/tools/codegen/optgen/internal/validator"
	"github.com/altessa-s/go-atlas/tools/codegen/optgen/plugin"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
	optconfig "github.com/altessa-s/go-atlas/tools/codegen/optgen/config"
	optparser "github.com/altessa-s/go-atlas/tools/codegen/optgen/internal/parser"
)

const (
	defaultOutputFile     = "options_gen.go"
	defaultTypeName       = "options"
	defaultOptionType     = "Option"
	defaultFilePermission = 0o644
)

// allowedFormatters is the allowlist of permitted formatter commands.
// Only these commands (or their full paths) are allowed to be executed.
var allowedFormatters = map[string]bool{
	"gofmt":     true,
	"goimports": true,
	"gofumpt":   true,
	"golines":   true,
	"crlfmt":    true,
	"gci":       true,
}

// GenerateCommand implements the "generate" subcommand.
//
// It parses the target directory for Go source files, locates the struct type
// identified by --type, collects opt-tagged fields, runs the plugin pipeline,
// and writes the generated WithXxx functions to --output (default options_gen.go).
// Use --dry-run to preview output without writing files.
type GenerateCommand struct {
	*cobra.Command
	typeName         string
	output           string
	optionType       string
	optionError      bool
	directory        string
	skipOptionType   bool
	skipDefaultFunc  bool
	skipNewFunc      bool
	dryRun           bool
	verbose          bool
	disablePlugins   string
	externalPlugins  []string
	defaultFunc      string
	newFunc          string
	optionPrefix     string
	processAllFields bool
	noConfig         bool   // Skip loading .optgen.yaml config file
	formatter        string // Formatter command (default: "gofmt -w")
	noFormat         bool   // Disable formatting
	generator        *generator.Generator
	loadedConfig     *optconfig.Config // Config loaded from .optgen.yaml
	projectRoot      string            // Root directory where config was found
}

// NewGenerate returns a cobra command that generates functional option functions
// from struct field tags. The returned command validates inputs in PersistentPreRunE
// (directory existence, identifier validity, config loading) before running generation.
func NewGenerate() *cobra.Command {
	cmd := &GenerateCommand{
		Command: &cobra.Command{
			Use:   "generate",
			Short: "Generate functional option functions",
			Long: heredoc.Doc(`
				Generate functional option functions from struct field tags.

				The generator scans the specified directory (default: current directory)
				for Go files containing struct types with 'opt' tags, then generates
				WithXxx functions for each tagged field.

				The 'opt' tag format:
				  opt:"Name"  - Generate option
				  opt:"-"     - Skip generation

				Where:
				  - Name: The option name (used in WithName function).
				  - Use optgen/optval/optcheck tags for defaults/modifiers/validations.

				The generator creates a file with:
				  - WithXxx functions for each tagged field
				  - defaultOptions() function with default values
				  - newOptions() function to create instances with options
			`),
			Example: heredoc.Doc(`
				# Generate options for 'options' struct type in current directory
				optgen generate

				# Generate with custom type name
				optgen generate --type config

				# Generate with custom output file
				optgen generate --output my_options.go

				# Generate with custom option type name
				optgen generate --option-type ConfigOption

				# Generate for specific directory
				optgen generate --directory ./mypackage

				# Preview what would be generated without writing files
				optgen generate --dry-run

				# Show detailed generation progress
				optgen generate --verbose

				# Combine dry-run with verbose for maximum visibility
				optgen generate --dry-run --verbose
			`),
		},
	}

	cmd.configure()

	return cmd.Command
}

// configure sets up the generate command with flags and run function.
func (c *GenerateCommand) configure() {
	c.Flags().StringVarP(&c.typeName, "type", "t", defaultTypeName, heredoc.Doc(`
		Name of the options struct type to generate functions for.
		The generator will search for this type in the specified directory.
	`))

	c.Flags().StringVarP(&c.output, "output", "o", "", heredoc.Doc(`
		Output file name for generated code.
		Default: options_gen.go
	`))

	c.Flags().StringVar(&c.optionType, "option-type", defaultOptionType, heredoc.Doc(`
		Name of the Option type (function signature).
		This will be used as the return type of generated WithXxx functions.
	`))

	c.Flags().BoolVar(&c.optionError, "option-error", false, heredoc.Doc(`
		Generate Option type as func(*T) error and newOptions returning (*T, error).
		This is useful when some options may validate input and return errors.
	`))

	c.Flags().StringVarP(&c.directory, "directory", "d", ".", heredoc.Doc(`
		Directory to process.
		The generator will scan this directory for Go files containing
		the specified struct type with opt tags.
	`))

	c.Flags().BoolVar(&c.skipOptionType, "skip-option-type", false, heredoc.Doc(`
		Skip generating the Option type definition in the generated file.
		By default, the generator includes:
		  type Option func(o *options)
		Use this flag if you want to define the Option type manually.
	`))

	c.Flags().BoolVar(&c.skipDefaultFunc, "skip-default-func", false, heredoc.Doc(`
		Skip generating the defaultOptions function in the generated file.
		Use this flag if you want to define the default function manually.
	`))

	c.Flags().BoolVar(&c.skipNewFunc, "skip-new-func", false, heredoc.Doc(`
		Skip generating the newOptions function in the generated file.
		Use this flag if you want to define the new function manually.
	`))

	c.Flags().BoolVar(&c.dryRun, "dry-run", false, heredoc.Doc(`
		Show what would be generated without writing to files.
		Useful for previewing changes before committing them.
	`))

	c.Flags().BoolVarP(&c.verbose, "verbose", "v", false, heredoc.Doc(`
		Enable verbose output showing detailed generation progress.
		Displays information about parsed fields and imports used.
	`))

	c.Flags().StringVar(&c.disablePlugins, "disable-plugins", "", heredoc.Doc(`
		Comma-separated list of plugin names to disable.
		Plugins are identified by their type name and are matched case-insensitively.
		Use this to disable specific plugins if they interfere with your code generation.
		Example: --disable-plugins=LoggerPlugin,NetIPParsePlugin
	`))

	c.Flags().StringArrayVar(&c.externalPlugins, "plugin", nil, heredoc.Doc(`
		Load external optgen plugins from Go .so files (Go buildmode=plugin).
		Each plugin's init() should register FieldPlugin/TypeDefaultProvider/CheckPlugin via optgen/plugin.

		Example:
		  optgen generate --plugin ./myoptgen.so

		Note: supported only on darwin/linux; the plugin must be built with the same Go version and module deps.
	`))

	c.Flags().StringVar(&c.defaultFunc, "default-func-name", "", heredoc.Doc(`
		Name of the function that returns default values.
		By default, uses "default<TypeName>" pattern (e.g., defaultOptions, defaultVerifierOptions).
		Example: --default-func-name=myCustomDefaultFunc
	`))

	c.Flags().StringVar(&c.newFunc, "new-func-name", "", heredoc.Doc(`
		Name of the function that creates new instances with options.
		By default, uses "new<TypeName>" pattern (e.g., newOptions, newVerifierOptions).
		Example: --new-func-name=myCustomNewFunc
	`))

	c.Flags().StringVar(&c.optionPrefix, "option-prefix", "", heredoc.Doc(`
		Prefix to add to all generated option function names.
		For field "issuer" with --option-prefix=Validation:
		  - Without prefix: WithIssuer
		  - With prefix:    WithValidationIssuer
		This is useful when generating validation options or other specialized option sets.
	`))

	c.Flags().BoolVar(&c.processAllFields, "all-fields", false, heredoc.Doc(`
		Process all struct fields, not just those with opt/optgen/optval/optcheck tags.
		Fields with opt:"-" are still skipped.
		Useful with --option-prefix to generate options for all fields without adding tags.
	`))

	c.Flags().BoolVar(&c.noConfig, "no-config", false, heredoc.Doc(`
		Skip loading .optgen.yaml configuration file.
		By default, optgen searches for .optgen.yaml in the current directory
		and parent directories up to the project root (go.mod or .git).
	`))

	c.Flags().StringVar(&c.formatter, "formatter", "gofmt -w", heredoc.Doc(`
		Command to format generated files. The file path is appended to the command.
		Set to empty string or use --no-format to disable formatting.

		For security, only allowlisted formatters are permitted:
		  gofmt, goimports, gofumpt, golines, crlfmt

		Shell operators (|, &&, ;, etc.) are not allowed.
		Examples: "gofmt -w", "goimports -w", "gofumpt -w"
	`))

	c.Flags().BoolVar(&c.noFormat, "no-format", false, heredoc.Doc(`
		Disable formatting of generated files.
		By default, generated files are formatted with "gofmt -w".
	`))

	// Set pre-run hook for validation
	c.PersistentPreRunE = c.preRun

	c.RunE = c.run
}

func (c *GenerateCommand) preRun(cmd *cobra.Command, _ []string) error {
	c.generator = generator.New()

	plugin.SetWarningSink(func(format string, args ...any) {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "optgen: WARN: "+format+"\n", args...)
	})

	// Load config file if not disabled
	if !c.noConfig {
		if err := c.loadConfig(cmd); err != nil {
			return err
		}
	}

	// Load external plugins (from config and CLI)
	allPlugins := c.externalPlugins
	if c.loadedConfig != nil && len(c.loadedConfig.Defaults.Plugins) > 0 {
		// Prepend config plugins, CLI plugins take precedence
		allPlugins = append(c.loadedConfig.Defaults.Plugins, allPlugins...)
	}
	if err := loadExternalPlugins(allPlugins); err != nil {
		return err
	}

	// Collect disabled plugins from config and CLI
	disabledPlugins := c.disablePlugins
	if c.loadedConfig != nil && !cmd.Flags().Changed("disable-plugins") {
		if len(c.loadedConfig.Defaults.DisablePlugins) > 0 {
			disabledPlugins = strings.Join(c.loadedConfig.Defaults.DisablePlugins, ",")
		}
	}

	if disabledPlugins != "" {
		pluginNames := strings.Split(disabledPlugins, ",")
		normalized := make([]string, 0, len(pluginNames))
		for i := range pluginNames {
			pluginNames[i] = strings.TrimSpace(pluginNames[i])
			if pluginNames[i] != "" {
				normalized = append(normalized, pluginNames[i])
			}
		}
		known := plugin.KnownPluginNames()
		if len(known) > 0 {
			knownLower := make(map[string]bool, len(known))
			for _, n := range known {
				knownLower[strings.ToLower(n)] = true
			}
			for _, n := range normalized {
				if !knownLower[strings.ToLower(n)] {
					return fmt.Errorf("unknown plugin %q (known: %s)", n, strings.Join(known, ", "))
				}
			}
		}
		c.generator.DisablePlugins(normalized...)
	}

	if info, err := os.Stat(c.directory); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("directory does not exist: %s", c.directory)
		}
		return coreerrs.Wrapf(err, "failed to access directory %s", c.directory)
	} else if !info.IsDir() {
		return fmt.Errorf("path is not a directory: %s", c.directory)
	}

	if c.typeName == "" {
		return fmt.Errorf("type name cannot be empty")
	}

	if err := validator.ValidateGoIdentifier(c.typeName); err != nil {
		return fmt.Errorf("invalid type name: %w", err)
	}

	if c.optionType == "" {
		return fmt.Errorf("option type cannot be empty")
	}

	if err := validator.ValidateGoIdentifier(c.optionType); err != nil {
		return fmt.Errorf("invalid option type: %w", err)
	}

	// Set default function names based on type name if not provided
	if c.defaultFunc == "" {
		c.defaultFunc = "default" + capitalizeFirst(c.typeName)
	}

	if c.newFunc == "" {
		c.newFunc = "new" + capitalizeFirst(c.typeName)
	}

	if err := validator.ValidateGoIdentifier(c.defaultFunc); err != nil {
		return fmt.Errorf("invalid default function name: %w", err)
	}

	if err := validator.ValidateGoIdentifier(c.newFunc); err != nil {
		return fmt.Errorf("invalid new function name: %w", err)
	}
	if c.defaultFunc == c.newFunc {
		return fmt.Errorf("default and new function names must differ (both are %q)", c.defaultFunc)
	}

	return nil
}

// capitalizeFirst capitalizes the first letter of a string.
func capitalizeFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// loadConfig loads the .optgen.yaml config file and applies defaults.
func (c *GenerateCommand) loadConfig(cmd *cobra.Command) error {
	// Find config file starting from the target directory
	configPath, projectRoot := optconfig.FindConfigFile(c.directory)
	c.projectRoot = projectRoot

	if configPath == "" {
		// No config file found, that's OK
		return nil
	}

	cfg, err := optconfig.Load(configPath)
	if err != nil {
		return coreerrs.Wrapf(err, "failed to load config %s", configPath)
	}
	c.loadedConfig = cfg

	if c.verbose {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Loaded config from: %s\n", configPath)
	}

	// Get package-specific config (or defaults)
	// Calculate relative path from project root to target directory
	relPath := c.directory
	if c.projectRoot != "" {
		absDir, errDir := filepath.Abs(c.directory)
		absRoot, errRoot := filepath.Abs(c.projectRoot)
		if errDir == nil && errRoot == nil {
			if rel, err := filepath.Rel(absRoot, absDir); err == nil {
				relPath = "./" + rel
			}
		}
	}
	pkgCfg := cfg.GetPackageConfig(relPath)

	// Apply config defaults for flags not explicitly set on CLI
	c.applyConfigDefaults(cmd, pkgCfg)

	return nil
}

// applyConfigDefaults applies config values to flags that weren't explicitly set.
func (c *GenerateCommand) applyConfigDefaults(cmd *cobra.Command, cfg optconfig.Defaults) {
	flags := cmd.Flags()

	// String flags
	if !flags.Changed("type") && cfg.Type != "" {
		c.typeName = cfg.Type
	}
	if !flags.Changed("output") && cfg.Output != "" {
		c.output = cfg.Output
	}
	if !flags.Changed("option-type") && cfg.OptionType != "" {
		c.optionType = cfg.OptionType
	}
	if !flags.Changed("default-func-name") && cfg.DefaultFuncName != "" {
		c.defaultFunc = cfg.DefaultFuncName
	}
	if !flags.Changed("new-func-name") && cfg.NewFuncName != "" {
		c.newFunc = cfg.NewFuncName
	}
	if !flags.Changed("option-prefix") && cfg.OptionPrefix != "" {
		c.optionPrefix = cfg.OptionPrefix
	}

	// Bool flags
	if !flags.Changed("option-error") && cfg.OptionError {
		c.optionError = cfg.OptionError
	}
	if !flags.Changed("all-fields") && cfg.AllFields {
		c.processAllFields = cfg.AllFields
	}
	if !flags.Changed("skip-option-type") && cfg.SkipOptionType {
		c.skipOptionType = cfg.SkipOptionType
	}
	if !flags.Changed("skip-default-func") && cfg.SkipDefaultFunc {
		c.skipDefaultFunc = cfg.SkipDefaultFunc
	}
	if !flags.Changed("skip-new-func") && cfg.SkipNewFunc {
		c.skipNewFunc = cfg.SkipNewFunc
	}
	if !flags.Changed("verbose") && cfg.Verbose {
		c.verbose = cfg.Verbose
	}
	if !flags.Changed("formatter") && cfg.Formatter != "" {
		c.formatter = cfg.Formatter
	}
	if !flags.Changed("no-format") && cfg.NoFormat {
		c.noFormat = cfg.NoFormat
	}
}

// filePlaceholder is the placeholder for the file path in formatter commands.
const filePlaceholder = "{file}"

// validateFormatter checks that the formatter command uses an allow-listed executable
// and contains no shell meta-characters (pipes, semicolons, backticks, etc.).
//
// It returns the executable name, parsed arguments, whether the "{file}" placeholder
// appears in the command string, and any validation error. An empty executable with
// a nil error means the formatter string was empty and formatting should be skipped.
func validateFormatter(formatter string) (executable string, args []string, hasFilePlaceholder bool, err error) {
	// Check for shell execution patterns - these indicate attempts to run shell commands
	// Note: quotes are allowed since we use exec.Command directly (no shell interpretation)
	dangerousPatterns := []string{"|", "&&", "||", ";", "`", "$(", ">", "<", "\n"}
	for _, pattern := range dangerousPatterns {
		if strings.Contains(formatter, pattern) {
			return "", nil, false, fmt.Errorf("formatter contains disallowed pattern %q; use simple commands like 'gofmt -w'", pattern)
		}
	}

	// Check if {file} placeholder is used
	hasFilePlaceholder = strings.Contains(formatter, filePlaceholder)

	// Parse the formatter string, handling quoted arguments
	parts := parseFormatterArgs(formatter)
	if len(parts) == 0 {
		return "", nil, false, nil // Empty formatter, will be skipped
	}

	executable = parts[0]
	args = parts[1:]

	// Extract base name for allowlist check (handles full paths like /usr/bin/gofmt)
	baseName := filepath.Base(executable)

	if !allowedFormatters[baseName] {
		allowed := make([]string, 0, len(allowedFormatters))
		for name := range allowedFormatters {
			allowed = append(allowed, name)
		}
		return "", nil, false, fmt.Errorf("formatter %q is not in the allowlist; allowed formatters: %s", baseName, strings.Join(allowed, ", "))
	}

	return executable, args, hasFilePlaceholder, nil
}

// parseFormatterArgs splits a formatter string into arguments, respecting quotes.
// Supports both double and single quotes. Quotes are removed from the result.
func parseFormatterArgs(s string) []string {
	var args []string
	var current strings.Builder
	var inQuote rune // 0 if not in quote, otherwise the quote character

	for _, r := range s {
		switch {
		case inQuote != 0:
			if r == inQuote {
				// End of quoted section
				inQuote = 0
			} else {
				current.WriteRune(r)
			}
		case r == '"' || r == '\'':
			// Start of quoted section
			inQuote = r
		case r == ' ' || r == '\t':
			// Whitespace outside quotes - end of argument
			if current.Len() > 0 {
				args = append(args, current.String())
				current.Reset()
			}
		default:
			current.WriteRune(r)
		}
	}

	// Don't forget the last argument
	if current.Len() > 0 {
		args = append(args, current.String())
	}

	return args
}

// formatFile formats the generated file using the configured formatter.
func (c *GenerateCommand) formatFile(cmd *cobra.Command, filePath string) error {
	// Skip formatting if disabled
	if c.noFormat || c.formatter == "" {
		return nil
	}

	// Validate the formatter against the allowlist
	executable, args, hasFilePlaceholder, err := validateFormatter(c.formatter)
	if err != nil {
		return fmt.Errorf("invalid formatter: %w", err)
	}
	if executable == "" {
		return nil // Empty formatter
	}

	// Handle file path: either replace {file} placeholder or append to args
	if hasFilePlaceholder {
		for i, arg := range args {
			args[i] = strings.ReplaceAll(arg, filePlaceholder, filePath)
		}
	} else {
		args = append(args, filePath)
	}

	if c.verbose {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Formatting with: %s %s\n", executable, strings.Join(args, " "))
	}

	// Execute the formatter directly without shell
	// #nosec G204 -- formatter executable resolved from trusted codegen config, not user input
	fmtCmd := exec.CommandContext(cmd.Context(), executable, args...)
	fmtCmd.Stdout = cmd.OutOrStdout()
	fmtCmd.Stderr = cmd.ErrOrStderr()

	if err := fmtCmd.Run(); err != nil {
		return coreerrs.Wrapf(err, "failed to format %s with %q", filePath, c.formatter)
	}

	return nil
}

func (c *GenerateCommand) run(cmd *cobra.Command, _ []string) error {
	const previewBytes = 1000

	if c.verbose {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Parsing directory: %s\n", c.directory)
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Looking for type: %s\n", c.typeName)
	}

	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, c.directory, func(fi os.FileInfo) bool { //nolint:staticcheck // SA1019: parser.ParseDir migration tracked separately
		return !strings.HasSuffix(fi.Name(), "_test.go") &&
			!strings.HasSuffix(fi.Name(), "_gen.go")
	}, parser.ParseComments)
	if err != nil {
		return coreerrs.WrapOperation(err, "parse directory")
	}

	if len(pkgs) == 0 {
		return fmt.Errorf("no packages found in %s", c.directory)
	}

	if c.verbose {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Found %d package(s)\n", len(pkgs))
	}

	generated := false
	for pkgName, pkg := range pkgs {
		if c.verbose {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Processing package: %s\n", pkgName)
		}

		result, err := optparser.FindOptFields(pkg, c.typeName, c.processAllFields)
		if err != nil {
			return coreerrs.WrapOperation(err, "find opt fields")
		}
		if len(result.Fields) == 0 {
			if c.verbose {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "No opt fields found in package %s\n", pkgName)
			}
			continue
		}

		// Check for normalization method on the struct
		normalizationMethod := optparser.FindNormalizationMethod(pkg, c.typeName)
		if c.verbose && normalizationMethod != "" {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Found normalization method: %s\n", normalizationMethod)
		}

		// Apply option prefix if specified
		if c.optionPrefix != "" {
			for i := range result.Fields {
				result.Fields[i].OptionName = c.optionPrefix + result.Fields[i].OptionName
			}
		}

		if c.verbose {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Found %d field(s) with opt tags:\n", len(result.Fields))
			for _, field := range result.Fields {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  - %s (%s) -> With%s\n", field.FieldName, field.Type, field.OptionName)
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Required imports: %v\n", result.Imports)
			if result.GenericInfo.IsGeneric() {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Generic type parameters: %s\n", result.GenericInfo.TypeParamsDecl())
			}
		}

		outputFile := c.output
		if outputFile == "" {
			outputFile = defaultOutputFile
		}

		if c.verbose {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Generating code...\n")
		}

		content, err := c.generator.Generate(generator.GenerateInput{
			PackageName:         pkgName,
			TypeName:            c.typeName,
			OptionType:          c.optionType,
			GenerateOptionType:  !c.skipOptionType,
			GenerateDefaultFunc: !c.skipDefaultFunc,
			GenerateNewFunc:     !c.skipNewFunc,
			OptionReturnsError:  c.optionError,
			Fields:              result.Fields,
			Imports:             result.Imports,
			DefaultFuncName:     c.defaultFunc,
			NewFuncName:         c.newFunc,
			NormalizationMethod: normalizationMethod,
			GenericInfo:         result.GenericInfo,
		})
		if err != nil {
			return coreerrs.WrapOperation(err, "generate")
		}

		outputPath := filepath.Join(c.directory, outputFile)

		if c.dryRun {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "DRY RUN: Would generate %s with %d option(s)\n", outputPath, len(result.Fields))
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "\n--- Generated content preview (first %d bytes) ---\n", previewBytes)
			preview := content
			if len(preview) > previewBytes {
				preview = content[:previewBytes]
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s\n...\n(truncated, total size: %d bytes)\n", preview, len(content))
			} else {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s\n", preview)
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "--- End preview ---\n")
		} else {
			if c.verbose {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Writing to: %s\n", outputPath)
			}

			if err := os.WriteFile(outputPath, content, defaultFilePermission); err != nil {
				return coreerrs.Wrapf(err, "failed to write output to %s", outputPath)
			}

			// Format the generated file
			if err := c.formatFile(cmd, outputPath); err != nil {
				return err
			}

			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Generated %s with %d option(s)\n", outputPath, len(result.Fields))
		}

		generated = true
	}

	if !generated {
		return fmt.Errorf("no struct with name '%s' and opt tags found", c.typeName)
	}

	return nil
}
