// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package appinfo

import (
	"bytes"
	"fmt"
	"runtime/debug"
	"slices"
	"strings"
	"text/template"

	coreslices "github.com/altessa-s/go-atlas/core/collections/slices"
)

//nolint:gochecknoglobals
var (
	// Name is the application name, typically the binary or service name.
	// It is set via linker flags during the build process:
	//
	//	go build -ldflags "-X 'appinfo.Name=MyApp'"
	//
	// If not set at build time, it defaults to the last path component of
	// the Go module path, or "unknown" if build info is unavailable.
	Name = ""

	// Project is the umbrella project name, used to group related applications
	// and construct filesystem paths (e.g., /etc/<Project>/<Name>).
	// It is set via linker flags during the build process:
	//
	//	go build -ldflags "-X 'appinfo.Project=MyProject'"
	Project = ""

	// Version is the application's semantic version string (e.g., "1.2.3" or "1.0.0-beta.1").
	// It defaults to "0.0.0" and is set via linker flags:
	//
	//	go build -ldflags "-X 'appinfo.Version=1.0.0'"
	//
	// The version is parsed at init time into a [SemanticVersion] accessible via [SemVersion].
	Version = "0.0.0"

	// BuildTime is the timestamp when the binary was built, typically in RFC 3339 format.
	// It is set via linker flags:
	//
	//	go build -ldflags "-X 'appinfo.BuildTime=2021-01-01T00:00:00Z'"
	//
	// If not set at build time, the value is populated from the vcs.time build setting.
	BuildTime = ""

	// EnvPrefix is the uppercase prefix prepended to environment variable keys
	// by [GetEnvVar]. Hyphens and dots are replaced with underscores at init time.
	// It is set via linker flags:
	//
	//	go build -ldflags "-X 'appinfo.EnvPrefix=MYAPP'"
	EnvPrefix = ""

	// Commit is the abbreviated (6 character), uppercase VCS revision hash.
	// It is set via linker flags:
	//
	//	go build -ldflags "-X 'appinfo.Commit=abcdef123456'"
	//
	// If not set at build time, the value is derived from vcs.revision in
	// the build info, truncated to 6 characters, and uppercased.
	Commit = ""

	// Branch is the VCS branch name from which the binary was built.
	// It is set via linker flags:
	//
	//	go build -ldflags "-X 'appinfo.Branch=main'"
	Branch = ""

	// StandaloneLabel is the human-readable label returned by [EnvLabel] when
	// the binary is built with the "standalone" build tag ([IsStandalone] is true).
	// Override via linker flags if a custom label is needed.
	StandaloneLabel = "standalone"

	// NonStandaloneLabel is the human-readable label returned by [EnvLabel] when
	// the binary is built without the "standalone" build tag ([IsStandalone] is false).
	// Override via linker flags if a custom label is needed.
	NonStandaloneLabel = "cloud"

	// BuildTags is the build tags used to build the application.
	buildTags = ""

	// GoVersion is Go tree's version string.
	goVersion = ""
	goOS      = ""
	goArch    = ""

	cgoEnabled  bool
	raceEnabled bool

	sver *SemanticVersion

	deps []*Dependency
)

// init initializes the package by attempting to read build information
// and setting default values for various fields if they aren't explicitly set.
func init() {
	buildInfo, ok := debug.ReadBuildInfo()
	if ok {
		deps = slices.Collect(coreslices.Map(buildInfo.Deps, func(i *debug.Module) *Dependency {
			dep := &Dependency{
				Path:    i.Path,
				Version: i.Version,
				Sum:     i.Sum,
			}

			if i.Replace != nil {
				dep.IsReplaced = true
				dep.ReplacedPath = &i.Replace.Path
				dep.Version = i.Replace.Version
				dep.Sum = i.Replace.Sum
			}

			return dep
		}))

		goVersion = buildInfo.GoVersion

		for _, setting := range buildInfo.Settings {
			switch {
			case setting.Key == "vcs.revision" && Commit == "":
				Commit = setting.Value
				const commitPrefixLength = 6
				if len(Commit) >= commitPrefixLength {
					Commit = Commit[0:commitPrefixLength]
				}
				Commit = strings.ToUpper(Commit)
			case setting.Key == "vcs.time" && BuildTime == "":
				BuildTime = setting.Value
			case setting.Key == "-tags":
				buildTags = setting.Value
			case setting.Key == "CGO_ENABLED":
				cgoEnabled = isTrue(setting.Value)
			case setting.Key == "-race":
				raceEnabled = isTrue(setting.Value)
			case setting.Key == "GOOS":
				goOS = setting.Value
			case setting.Key == "GOARCH":
				goArch = setting.Value
			}
		}
	}

	if Name == "" {
		if !ok {
			// Graceful fallback: use "unknown" instead of panicking
			// This allows the application to continue running even if build info is unavailable
			Name = "unknown"
		} else {
			pathComps := strings.Split(buildInfo.Path, "/")
			if len(pathComps) > 0 {
				Name = pathComps[len(pathComps)-1]
			}
			// If pathComps is empty, Name remains empty and will be set to "unknown" below
			if Name == "" {
				Name = "unknown"
			}
		}
	}

	if ver, err := parseSemVer(Version); err == nil {
		sver = ver
	}

	if EnvPrefix != "" {
		env := strings.ReplaceAll(EnvPrefix, "-", "_")
		env = strings.ReplaceAll(env, ".", "_")

		EnvPrefix = strings.ToUpper(env)
	}
}

// IsPreRelease reports whether the current [Version] contains pre-release
// identifiers (e.g., "1.0.0-alpha.1"). It returns false if the version
// string could not be parsed as a valid semantic version.
func IsPreRelease() bool {
	return sver != nil && len(sver.Prerelease) > 0
}

// IsRelease reports whether the current [Version] is a stable release,
// meaning it is not an alpha, beta, or any other pre-release variant.
func IsRelease() bool {
	return !IsPreRelease() && !IsAlpha() && !IsBeta()
}

// IsReleaseCandidate reports whether the current [Version] is a release
// candidate, identified by "rc" as the first pre-release segment
// (e.g., "1.0.0-rc.1").
func IsReleaseCandidate() bool {
	return sver != nil && len(sver.Prerelease) > 0 && sver.Prerelease[0] == "rc"
}

// IsAlpha reports whether the current [Version] is an alpha release.
// A version is considered alpha if it equals the default "0.0.0" or has
// "alpha" as the first pre-release segment (e.g., "1.0.0-alpha.1").
func IsAlpha() bool {
	return Version == "0.0.0" || (sver != nil && len(sver.Prerelease) > 0 && sver.Prerelease[0] == "alpha")
}

// IsBeta reports whether the current [Version] is a beta release,
// identified by "beta" as the first pre-release segment (e.g., "1.0.0-beta.1").
func IsBeta() bool {
	return sver != nil && len(sver.Prerelease) > 0 && sver.Prerelease[0] == "beta"
}

// BuildTags returns the comma-separated build tags that were active when the
// binary was compiled (the value of the "-tags" build setting).
func BuildTags() string {
	return buildTags
}

// Deps returns the application's module dependencies as a [Dependencies] collection.
// The data is extracted from [debug.ReadBuildInfo] at init time and includes module
// paths, versions, checksums, and replacement information.
func Deps() Dependencies {
	return deps
}

// SemVersion returns a copy of the parsed [SemanticVersion] for the current [Version].
// If the version string could not be parsed, an empty SemanticVersion is returned.
// The returned value is a copy, so callers may safely modify it.
func SemVersion() *SemanticVersion {
	if sver == nil {
		return &SemanticVersion{}
	}

	return &SemanticVersion{
		Major:      sver.Major,
		Minor:      sver.Minor,
		Patch:      sver.Patch,
		Prerelease: sver.Prerelease,
	}
}

// IsCgoEnabled reports whether the binary was compiled with CGO support enabled
// (the CGO_ENABLED build setting was "true" or "1").
func IsCgoEnabled() bool {
	return cgoEnabled
}

// IsRaceEnabled reports whether the binary was compiled with the Go race
// detector enabled (the "-race" build setting was active).
func IsRaceEnabled() bool {
	return raceEnabled
}

// EnvLabel returns [StandaloneLabel] when the binary was built with the
// "standalone" build tag, or [NonStandaloneLabel] otherwise. The returned
// value is suitable for display in version strings and diagnostics.
func EnvLabel() string {
	if IsStandalone {
		return StandaloneLabel
	}
	return NonStandaloneLabel
}

// Info returns a compact, single-line summary of the application's version
// metadata in the format "version=X.Y.Z, revision=ABCDEF, env_prefix=PREFIX".
func Info() string {
	return fmt.Sprintf("version=%s, revision=%s, env_prefix=%s", Version, Commit, EnvPrefix)
}

// BuildInfo returns a compact, single-line summary of the build toolchain
// in the format "go=X.Y.Z, platform=os/arch, date=YYYY-MM-DD, tags=tag1,tag2".
func BuildInfo() string {
	return fmt.Sprintf("go=%s, platform=%s, date=%s, tags=%s", goVersion, goOS+"/"+goArch, BuildTime, buildTags)
}

// BuildGoVersion returns the Go toolchain version string (e.g., "go1.24.0")
// recorded in the binary's build info.
func BuildGoVersion() string {
	return goVersion
}

// BuildPlatform returns the target platform in "GOOS/GOARCH" format
// (e.g., "linux/amd64" or "darwin/arm64").
func BuildPlatform() string {
	return goOS + "/" + goArch
}

// BuildGoOS returns the GOOS value from the build settings
// (e.g., "linux", "darwin", "windows").
func BuildGoOS() string {
	return goOS
}

// BuildGoArch returns the GOARCH value from the build settings
// (e.g., "amd64", "arm64", "386").
func BuildGoArch() string {
	return goArch
}

// Template for formatted version information output
var versionInfoTmpl = `
{{.program}}, version {{.version}} (revision: {{.revision}})
  build date:       {{.buildDate}}
  build tags:       {{.tags}}
  go version:       {{.goVersion}}
  platform:         {{.platform}}
  env prefix:       {{.envPrefix}}
`

// AppVersion returns a multi-line, human-readable summary of the application's
// version, revision, build date, build tags, Go version, platform, and
// environment prefix. The output is intended for CLI --version flags and
// startup banners.
func AppVersion() string {
	m := map[string]string{
		"program":   Project,
		"version":   Version,
		"revision":  Commit,
		"branch":    Branch,
		"buildDate": BuildTime,
		"goVersion": goVersion,
		"platform":  goOS + "/" + goArch,
		"tags":      buildTags,
		"envLabel":  EnvLabel(),
		"envPrefix": EnvPrefix,
	}
	t := template.Must(template.New("version").Parse(versionInfoTmpl))

	var buf bytes.Buffer
	if err := t.ExecuteTemplate(&buf, "version", m); err != nil {
		return ""
	}
	return strings.TrimSpace(buf.String())
}

// SemanticVersion represents a parsed semantic version per the SemVer 2.0.0 specification.
// Use [SemVersion] to obtain the parsed version of the running application.
type SemanticVersion struct {
	Major      string   // Major version number; incremented for breaking changes.
	Minor      string   // Minor version number; incremented for backward-compatible additions.
	Patch      string   // Patch version number; incremented for backward-compatible fixes.
	Prerelease []string // Pre-release identifiers (e.g., ["alpha", "1"] for "1.0.0-alpha.1").
}

// Dependency describes a single Go module dependency extracted from
// [debug.ReadBuildInfo]. If the module is replaced via a go.mod replace
// directive, IsReplaced is true and ReplacedPath contains the replacement path.
type Dependency struct {
	Path         string  // Module path (e.g., "github.com/example/mod").
	ReplacedPath *string // Original module path before replacement, or nil.
	Version      string  // Module version or pseudo-version string.
	Sum          string  // Go checksum (go.sum hash).
	IsReplaced   bool    // True when the module is a replacement.
}

// Dependencies is a slice of [Dependency] pointers with convenience lookup methods.
// Obtain the application's dependencies via [Deps].
type Dependencies []*Dependency

// Contains reports whether any dependency in the collection has the given
// module path. The comparison is exact (case-sensitive, no prefix matching).
func (d Dependencies) Contains(path string) bool {
	for _, dep := range d {
		if dep.Path == path {
			return true
		}
	}
	return false
}

// Get returns the first [Dependency] whose Path matches the given module path,
// or nil if no match is found.
func (d Dependencies) Get(path string) *Dependency {
	for _, dep := range d {
		if dep.Path == path {
			return dep
		}
	}
	return nil
}

// isTrue returns true if the string value is "true" (case-insensitive) or "1".
// Used for parsing boolean values from build settings.
func isTrue(value string) bool {
	return strings.ToLower(value) == "true" || value == "1"
}
